package session

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"realityconverter/internal/converter"
)

const (
	startupTimeout = 12 * time.Second
	stopTimeout    = 5 * time.Second
)

type Event struct {
	Unexpected bool
	Error      error
	Log        string
}

type PortConflictError struct {
	Ports []int
}

func (err PortConflictError) Error() string {
	parts := make([]string, len(err.Ports))
	for index, port := range err.Ports {
		parts[index] = fmt.Sprintf("%d", port)
	}
	return "以下本地端口已被占用：" + strings.Join(parts, "、")
}

type Manager struct {
	mu         sync.Mutex
	process    *exec.Cmd
	job        windows.Handle
	done       chan struct{}
	expected   bool
	exitErr    error
	configPath string
	logs       *safeBuffer
	events     chan Event
}

func NewManager() *Manager {
	return &Manager{events: make(chan Event, 8)}
}

func (manager *Manager) Events() <-chan Event {
	return manager.events
}

func (manager *Manager) IsRunning() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.process != nil
}

func (manager *Manager) Start(ctx context.Context, xrayPath, dataDirectory string, configJSON []byte, result converter.ParseResult) error {
	manager.mu.Lock()
	if manager.process != nil {
		manager.mu.Unlock()
		return fmt.Errorf("Xray 已经在运行")
	}
	manager.mu.Unlock()

	ports := mappingPorts(result.Mappings)
	if conflicts := unavailablePorts(ports); len(conflicts) > 0 {
		return PortConflictError{Ports: conflicts}
	}
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		return fmt.Errorf("创建运行目录失败：%w", err)
	}
	temporary, err := os.CreateTemp(dataDirectory, ".running-config-*.json")
	if err != nil {
		return fmt.Errorf("创建运行配置失败：%w", err)
	}
	configPath := temporary.Name()
	if _, err := temporary.Write(configJSON); err != nil {
		_ = temporary.Close()
		_ = os.Remove(configPath)
		return fmt.Errorf("写入运行配置失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(configPath)
		return fmt.Errorf("关闭运行配置失败：%w", err)
	}

	checkContext, cancelCheck := context.WithTimeout(ctx, 30*time.Second)
	checkErr := (converter.ProcessChecker{}).Check(checkContext, xrayPath, configPath, result.Nodes)
	cancelCheck()
	if checkErr != nil {
		_ = os.Remove(configPath)
		return checkErr
	}

	job, err := createJob()
	if err != nil {
		_ = os.Remove(configPath)
		return fmt.Errorf("创建 Xray 进程隔离环境失败：%w", err)
	}
	logs := &safeBuffer{}
	command := exec.Command(xrayPath, "run", "-config", configPath)
	command.Dir = filepath.Dir(xrayPath)
	command.Stdout = logs
	command.Stderr = logs
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := command.Start(); err != nil {
		windows.CloseHandle(job)
		_ = os.Remove(configPath)
		return fmt.Errorf("启动 Xray 失败：%w", err)
	}
	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(command.Process.Pid),
	)
	if err == nil {
		err = windows.AssignProcessToJobObject(job, processHandle)
		windows.CloseHandle(processHandle)
	}
	if err != nil {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
		windows.CloseHandle(job)
		_ = os.Remove(configPath)
		return fmt.Errorf("将 Xray 加入受控进程失败：%w", err)
	}

	done := make(chan struct{})
	manager.mu.Lock()
	manager.process = command
	manager.job = job
	manager.done = done
	manager.expected = false
	manager.exitErr = nil
	manager.configPath = configPath
	manager.logs = logs
	manager.mu.Unlock()
	go manager.wait(command, job, done, configPath, logs, result.Nodes)

	readyContext, cancelReady := context.WithTimeout(ctx, startupTimeout)
	defer cancelReady()
	if err := waitForPorts(readyContext, ports, done); err != nil {
		_ = manager.Stop(context.Background())
		return fmt.Errorf("Xray 未能监听全部端口：%w\r\n%s", err, converter.Redact(logs.String(), result.Nodes))
	}
	_ = os.Remove(configPath)
	return nil
}

func (manager *Manager) Stop(ctx context.Context) error {
	manager.mu.Lock()
	if manager.process == nil {
		manager.mu.Unlock()
		return nil
	}
	manager.expected = true
	job := manager.job
	done := manager.done
	manager.mu.Unlock()

	if err := windows.TerminateJobObject(job, 0); err != nil {
		return fmt.Errorf("停止 Xray 失败：%w", err)
	}
	waitContext, cancel := context.WithTimeout(ctx, stopTimeout)
	defer cancel()
	select {
	case <-done:
		return nil
	case <-waitContext.Done():
		return fmt.Errorf("等待 Xray 停止超时")
	}
}

func (manager *Manager) wait(command *exec.Cmd, job windows.Handle, done chan struct{}, configPath string, logs *safeBuffer, nodes []converter.Node) {
	err := command.Wait()
	_ = os.Remove(configPath)
	windows.CloseHandle(job)

	manager.mu.Lock()
	expected := manager.expected
	if manager.process == command {
		manager.process = nil
		manager.job = 0
		manager.done = nil
		manager.configPath = ""
		manager.exitErr = err
	}
	manager.mu.Unlock()
	close(done)

	event := Event{Unexpected: !expected, Error: err, Log: converter.Redact(logs.String(), nodes)}
	select {
	case manager.events <- event:
	default:
	}
}

func createJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var information windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	information.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&information)),
		uint32(unsafe.Sizeof(information)),
	); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func mappingPorts(mappings []converter.Mapping) []int {
	ports := make([]int, len(mappings))
	for index, mapping := range mappings {
		ports[index] = mapping.ListenPort
	}
	return ports
}

func unavailablePorts(ports []int) []int {
	var conflicts []int
	for _, port := range ports {
		listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			conflicts = append(conflicts, port)
			continue
		}
		_ = listener.Close()
	}
	return conflicts
}

func waitForPorts(ctx context.Context, ports []int, processDone <-chan struct{}) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		allListening := true
		for _, port := range ports {
			connection, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), 150*time.Millisecond)
			if err != nil {
				allListening = false
				break
			}
			_ = connection.Close()
		}
		if allListening {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-processDone:
			return fmt.Errorf("Xray 进程已退出")
		case <-ticker.C:
		}
	}
}

type safeBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (buffer *safeBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Write(data)
}

func (buffer *safeBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}
