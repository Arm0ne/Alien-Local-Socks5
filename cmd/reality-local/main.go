package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"

	"realityconverter/internal/appcore"
	"realityconverter/internal/converter"
	"realityconverter/internal/ipcheck"
	"realityconverter/internal/profile"
	"realityconverter/internal/session"
)

var version = "dev"

const applicationName = "Alien Local Socks5"

type application struct {
	window            *walk.MainWindow
	importButton      *walk.PushButton
	startButton       *walk.PushButton
	stopButton        *walk.PushButton
	detectAllButton   *walk.PushButton
	detectOneButton   *walk.PushButton
	copyButton        *walk.PushButton
	deleteButton      *walk.PushButton
	startPort         *walk.NumberEdit
	table             *walk.TableView
	stateLabel        *walk.Label
	totalLabel        *walk.Label
	listeningLabel    *walk.Label
	normalLabel       *walk.Label
	failedLabel       *walk.Label
	detailLabel       *walk.Label
	model             *portTableModel
	service           *appcore.Service
	result            converter.ParseResult
	configured        bool
	running           bool
	busy              bool
	closing           bool
	checkCancel       context.CancelFunc
	applicationCtx    context.Context
	applicationCancel context.CancelFunc
}

func main() {
	instanceHandle, err := acquireSingleInstance()
	if err != nil {
		walk.MsgBox(nil, applicationName, err.Error(), walk.MsgBoxIconInformation)
		return
	}
	defer windows.CloseHandle(instanceHandle)

	service, err := appcore.New()
	if err != nil {
		walk.MsgBox(nil, applicationName, err.Error(), walk.MsgBoxIconError)
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	app := &application{
		model:             &portTableModel{},
		service:           service,
		applicationCtx:    ctx,
		applicationCancel: cancel,
	}
	if err := app.run(); err != nil {
		cancel()
		walk.MsgBox(nil, applicationName, err.Error(), walk.MsgBoxIconError)
		os.Exit(1)
	}
}

func acquireSingleInstance() (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(`Local\AlienLocalSocks5-4B59BC96-36D0-4E7B-98F4-E434E8A12713`)
	if err != nil {
		return 0, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return 0, fmt.Errorf("%s 已经在运行", applicationName)
	}
	if err != nil {
		return 0, fmt.Errorf("创建软件实例失败：%w", err)
	}
	return handle, nil
}

func (app *application) run() error {
	icon, err := walk.NewIconFromResourceId(2)
	if err != nil {
		return fmt.Errorf("加载应用图标失败：%w", err)
	}
	defer icon.Dispose()

	window := MainWindow{
		AssignTo: &app.window,
		Icon:     icon,
		Title:    applicationName,
		Size:     Size{Width: 900, Height: 620},
		MinSize:  Size{Width: 760, Height: 520},
		Font:     Font{Family: "Microsoft YaHei UI", PointSize: 9},
		Layout:   VBox{Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 12}, Spacing: 10},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 10},
				Children: []Widget{
					Label{Text: applicationName, Font: Font{Family: "Microsoft YaHei UI", PointSize: 14, Bold: true}},
					Label{AssignTo: &app.stateLabel, Text: "未导入节点", TextColor: walk.RGB(90, 90, 90)},
					HSpacer{},
					Label{Text: "起始端口"},
					NumberEdit{
						AssignTo:           &app.startPort,
						MinValue:           1,
						MaxValue:           65535,
						Decimals:           0,
						Increment:          1,
						SpinButtonsVisible: true,
						MinSize:            Size{Width: 105},
					},
					Label{Text: "v" + version, TextColor: walk.RGB(110, 110, 110)},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					PushButton{AssignTo: &app.importButton, Text: "导入节点", MinSize: Size{Width: 96, Height: 32}, OnClicked: app.importNodes},
					PushButton{AssignTo: &app.deleteButton, Text: "删除节点", MinSize: Size{Width: 96, Height: 32}, OnClicked: app.deleteSelected},
					PushButton{AssignTo: &app.startButton, Text: "启动", MinSize: Size{Width: 82, Height: 32}, OnClicked: app.start},
					PushButton{AssignTo: &app.stopButton, Text: "停止", MinSize: Size{Width: 82, Height: 32}, OnClicked: app.stop},
					PushButton{AssignTo: &app.detectAllButton, Text: "重新检测全部", MinSize: Size{Width: 112, Height: 32}, OnClicked: app.detectAll},
					PushButton{AssignTo: &app.detectOneButton, Text: "检测选中端口", MinSize: Size{Width: 112, Height: 32}, OnClicked: app.detectSelected},
					PushButton{AssignTo: &app.copyButton, Text: "复制代理地址", MinSize: Size{Width: 104, Height: 32}, OnClicked: app.copySelected},
					HSpacer{},
				},
			},
			Composite{
				Layout: Grid{Columns: 4, MarginsZero: true, Spacing: 8},
				Children: []Widget{
					Label{Text: "端口总数", TextColor: walk.RGB(100, 100, 100), Alignment: AlignHCenterVCenter},
					Label{Text: "SOCKS5 已监听", TextColor: walk.RGB(100, 100, 100), Alignment: AlignHCenterVCenter},
					Label{Text: "出口检测成功", TextColor: walk.RGB(100, 100, 100), Alignment: AlignHCenterVCenter},
					Label{Text: "检测失败", TextColor: walk.RGB(100, 100, 100), Alignment: AlignHCenterVCenter},
					Label{AssignTo: &app.totalLabel, Text: "0", Font: Font{Family: "Microsoft YaHei UI", PointSize: 13, Bold: true}, Alignment: AlignHCenterVCenter},
					Label{AssignTo: &app.listeningLabel, Text: "0 / 0", Font: Font{Family: "Microsoft YaHei UI", PointSize: 13, Bold: true}, Alignment: AlignHCenterVCenter},
					Label{AssignTo: &app.normalLabel, Text: "0 / 0", Font: Font{Family: "Microsoft YaHei UI", PointSize: 13, Bold: true}, Alignment: AlignHCenterVCenter},
					Label{AssignTo: &app.failedLabel, Text: "0", Font: Font{Family: "Microsoft YaHei UI", PointSize: 13, Bold: true}, Alignment: AlignHCenterVCenter},
				},
			},
			TableView{
				AssignTo:                    &app.table,
				Model:                       app.model,
				AlternatingRowBG:            true,
				NotSortableByHeaderClick:    true,
				MultiSelection:              false,
				SelectionHiddenWithoutFocus: false,
				CustomRowHeight:             30,
				StretchFactor:               1,
				LastColumnStretched:         true,
				Columns: []TableViewColumn{
					{Title: "状态", Width: 130, Alignment: AlignCenter},
					{Title: "本地 SOCKS5", Width: 220},
					{Title: "出口 IP", Width: 260},
					{Title: "最后检测", Width: 160, Alignment: AlignCenter},
				},
				StyleCell:             app.styleCell,
				OnCurrentIndexChanged: app.showSelectedDetail,
				OnItemActivated:       app.copySelected,
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					Label{AssignTo: &app.detailLabel, Text: "等待导入节点", TextColor: walk.RGB(95, 95, 95)},
					HSpacer{},
				},
			},
		},
	}
	if err := window.Create(); err != nil {
		return err
	}
	if err := app.startPort.SetValue(converter.DefaultStartPort); err != nil {
		app.window.Dispose()
		return err
	}
	app.window.Closing().Attach(app.onClosing)
	app.updateControls()
	app.loadSavedProfile()
	go app.watchSessionEvents()
	app.window.Run()
	return nil
}

func (app *application) loadSavedProfile() {
	result, err := app.service.Load()
	if errors.Is(err, profile.ErrNotFound) {
		return
	}
	if err != nil {
		app.setDetail("已保存节点加载失败：" + err.Error())
		return
	}
	app.result = result
	app.configured = true
	if len(result.Mappings) > 0 {
		_ = app.startPort.SetValue(float64(result.Mappings[0].ListenPort))
	}
	app.setRows(result, statusReady)
	_ = app.stateLabel.SetText("配置就绪")
	app.setDetail(fmt.Sprintf("已加载 %d 个 SOCKS5 端口", len(result.Mappings)))
	app.updateControls()
}

func (app *application) importNodes() {
	dialog := &walk.FileDialog{Title: "导入 Reality 节点", Filter: "文本文件 (*.txt)|*.txt|所有文件 (*.*)|*.*"}
	accepted, err := dialog.ShowOpen(app.window)
	if err != nil {
		app.showError("打开文件选择窗口失败：" + err.Error())
		return
	}
	if !accepted {
		return
	}
	path := dialog.FilePath
	startPort := int(app.startPort.Value())
	app.beginBusy("正在导入并检查配置...")
	go func() {
		result, err := app.service.ImportFile(app.applicationCtx, path, startPort)
		app.synchronize(func() {
			app.endBusy()
			if err != nil {
				app.showError(err.Error())
				return
			}
			app.result = result
			app.configured = true
			app.setRows(result, statusReady)
			_ = app.stateLabel.SetText("配置就绪")
			app.setDetail(fmt.Sprintf("已导入 %d 个 SOCKS5 端口，配置检查通过", len(result.Mappings)))
			app.updateControls()
		})
	}()
}

func (app *application) deleteSelected() {
	if !app.configured || app.running || app.busy {
		return
	}
	index := app.table.CurrentIndex()
	if index < 0 || index >= len(app.result.Nodes) || index >= len(app.result.Mappings) {
		app.setDetail("请先选择一个节点")
		return
	}

	answer := walk.MsgBox(app.window, applicationName,
		fmt.Sprintf("确定删除选中的节点吗？\r\n本地 SOCKS5 端口 %d 将被移除。", app.result.Mappings[index].ListenPort),
		walk.MsgBoxYesNo|walk.MsgBoxIconWarning)
	if answer != 6 {
		return
	}

	startPort := int(app.startPort.Value())
	app.beginBusy("正在删除节点并检查配置...")
	go func() {
		result, err := app.service.RemoveNode(app.applicationCtx, index, startPort)
		app.synchronize(func() {
			app.endBusy()
			if err != nil {
				app.showError(err.Error())
				return
			}
			app.result = result
			app.configured = len(result.Nodes) > 0
			app.setRows(result, statusReady)
			_ = app.table.SetCurrentIndex(-1)
			if app.configured {
				_ = app.stateLabel.SetText("配置就绪")
				app.setDetail(fmt.Sprintf("已删除节点，剩余 %d 个 SOCKS5 端口，配置检查通过", len(result.Mappings)))
			} else {
				_ = app.stateLabel.SetText("未导入节点")
				app.setDetail("已删除最后一个节点")
			}
			app.updateControls()
		})
	}()
}

func (app *application) start() {
	if !app.configured || app.busy || app.running {
		return
	}
	startPort := int(app.startPort.Value())
	app.cancelChecks()
	app.beginBusy("正在启动 Xray...")
	app.setAllStatus(statusStarting, "等待本地端口监听")
	go func() {
		result, err := app.service.Start(app.applicationCtx, startPort)
		app.synchronize(func() {
			app.endBusy()
			if err != nil {
				app.handleStartError(err)
				return
			}
			app.result = result
			app.running = true
			app.setRows(result, statusListening)
			_ = app.stateLabel.SetText("运行中")
			app.setDetail(fmt.Sprintf("已开启 %d 个 SOCKS5 端口", len(result.Mappings)))
			app.updateControls()
			app.detectAll()
		})
	}()
}

func (app *application) handleStartError(err error) {
	app.setRows(app.result, statusStopped)
	var conflict session.PortConflictError
	if errors.As(err, &conflict) {
		for index := range app.model.snapshot() {
			row, _ := app.model.item(index)
			for _, port := range conflict.Ports {
				if row.Port == port {
					app.model.update(index, func(item *portRow) {
						item.Status = statusConflict
						item.Detail = fmt.Sprintf("本地端口 %d 已被其他程序占用", port)
					})
				}
			}
		}
		if recommendation := recommendStartPort(int(app.startPort.Value()), len(app.result.Mappings)); recommendation > 0 {
			message := fmt.Sprintf("%s\r\n\r\n推荐可用范围：%d-%d\r\n是否使用推荐端口？", err.Error(), recommendation, recommendation+len(app.result.Mappings)-1)
			answer := walk.MsgBox(app.window, "端口被占用", message, walk.MsgBoxYesNo|walk.MsgBoxIconWarning)
			if answer == 6 {
				_ = app.startPort.SetValue(float64(recommendation))
				app.setDetail("已采用推荐端口，请点击启动")
				app.updateSummary()
				return
			}
		}
	}
	_ = app.stateLabel.SetText("启动失败")
	app.showError(err.Error())
	app.updateControls()
}

func (app *application) stop() {
	if !app.running || app.busy {
		return
	}
	app.cancelChecks()
	app.beginBusy("正在停止 Xray...")
	go func() {
		err := app.service.Stop(app.applicationCtx)
		app.synchronize(func() {
			app.endBusy()
			app.running = false
			app.setRows(app.result, statusStopped)
			_ = app.stateLabel.SetText("已停止")
			if err != nil {
				app.showError(err.Error())
			} else {
				app.setDetail("所有本软件管理的 SOCKS5 端口已停止")
			}
			app.updateControls()
		})
	}()
}

func (app *application) detectAll() {
	if !app.running || app.busy {
		return
	}
	indexes := make([]int, len(app.model.snapshot()))
	for index := range indexes {
		indexes[index] = index
	}
	app.detect(indexes)
}

func (app *application) detectSelected() {
	if !app.running || app.busy {
		return
	}
	index := app.table.CurrentIndex()
	if index < 0 {
		app.setDetail("请先选择一个 SOCKS5 端口")
		return
	}
	app.detect([]int{index})
}

func (app *application) detect(indexes []int) {
	app.cancelChecks()
	ctx, cancel := context.WithCancel(app.applicationCtx)
	app.checkCancel = cancel
	for _, index := range indexes {
		app.model.update(index, func(item *portRow) {
			item.Status = statusChecking
			item.ExitIP = "检测中..."
			item.Detail = "正在通过该 SOCKS5 端口检测出口 IP"
		})
	}
	app.updateSummary()
	app.updateControls()
	app.setDetail(fmt.Sprintf("正在检测 %d 个出口", len(indexes)))

	go func() {
		semaphore := make(chan struct{}, 3)
		var waitGroup sync.WaitGroup
		for _, index := range indexes {
			row, exists := app.model.item(index)
			if !exists {
				continue
			}
			waitGroup.Add(1)
			go func(rowIndex int, port int) {
				defer waitGroup.Done()
				select {
				case semaphore <- struct{}{}:
					defer func() { <-semaphore }()
				case <-ctx.Done():
					return
				}
				checkContext, cancelCheck := context.WithTimeout(ctx, 15*time.Second)
				address, err := (ipcheck.Checker{}).Check(checkContext, port)
				cancelCheck()
				if ctx.Err() != nil {
					return
				}
				app.synchronize(func() {
					app.model.update(rowIndex, func(item *portRow) {
						item.CheckedAt = time.Now().Format("15:04:05")
						if err != nil {
							item.Status = statusCheckFailed
							item.ExitIP = "未获取"
							item.Detail = err.Error()
							return
						}
						item.Status = statusNormal
						item.ExitIP = address
						item.Detail = "出口 IP 检测成功"
					})
					app.updateSummary()
					app.showSelectedDetail()
				})
			}(index, row.Port)
		}
		waitGroup.Wait()
		if ctx.Err() == nil {
			app.synchronize(func() {
				app.setDetail("出口检测完成")
				app.updateControls()
			})
		}
	}()
}

func (app *application) copySelected() {
	row, exists := app.model.item(app.table.CurrentIndex())
	if !exists {
		app.setDetail("请先选择一个 SOCKS5 端口")
		return
	}
	if err := walk.Clipboard().SetText(row.Proxy); err != nil {
		app.showError("复制失败：" + err.Error())
		return
	}
	app.setDetail("已复制 " + row.Proxy)
}

func (app *application) watchSessionEvents() {
	for {
		select {
		case <-app.applicationCtx.Done():
			return
		case event := <-app.service.Events():
			if !event.Unexpected {
				continue
			}
			app.synchronize(func() {
				app.cancelChecks()
				app.running = false
				app.busy = false
				app.setRows(app.result, statusStopped)
				_ = app.stateLabel.SetText("Xray 已退出")
				detail := strings.TrimSpace(event.Log)
				if detail == "" && event.Error != nil {
					detail = event.Error.Error()
				}
				app.setDetail("Xray 意外退出：" + oneLine(detail))
				app.updateControls()
			})
		}
	}
}

func (app *application) setRows(result converter.ParseResult, status portStatus) {
	rows := make([]portRow, len(result.Mappings))
	for index, mapping := range result.Mappings {
		rows[index] = portRow{
			Status: status,
			Proxy:  fmt.Sprintf("127.0.0.1:%d", mapping.ListenPort),
			ExitIP: "-",
			Port:   mapping.ListenPort,
		}
	}
	app.model.reset(rows)
	app.updateSummary()
}

func (app *application) setAllStatus(status portStatus, detail string) {
	for index := range app.model.snapshot() {
		app.model.update(index, func(item *portRow) {
			item.Status = status
			item.ExitIP = "-"
			item.CheckedAt = ""
			item.Detail = detail
		})
	}
	app.updateSummary()
}

func (app *application) updateSummary() {
	rows := app.model.snapshot()
	listening := 0
	normal := 0
	failed := 0
	for _, row := range rows {
		switch row.Status {
		case statusListening, statusChecking, statusNormal, statusCheckFailed:
			listening++
		}
		if row.Status == statusNormal {
			normal++
		}
		if row.Status == statusCheckFailed {
			failed++
		}
	}
	_ = app.totalLabel.SetText(fmt.Sprintf("%d", len(rows)))
	_ = app.listeningLabel.SetText(fmt.Sprintf("%d / %d", listening, len(rows)))
	_ = app.normalLabel.SetText(fmt.Sprintf("%d / %d", normal, len(rows)))
	_ = app.failedLabel.SetText(fmt.Sprintf("%d", failed))
}

func (app *application) updateControls() {
	app.importButton.SetEnabled(!app.running && !app.busy)
	app.deleteButton.SetEnabled(app.configured && !app.running && !app.busy)
	app.startButton.SetEnabled(app.configured && !app.running && !app.busy)
	app.stopButton.SetEnabled(app.running && !app.busy)
	app.startPort.SetEnabled(!app.running && !app.busy)
	detectEnabled := app.running && !app.busy
	app.detectAllButton.SetEnabled(detectEnabled)
	app.detectOneButton.SetEnabled(detectEnabled)
	app.copyButton.SetEnabled(app.configured && app.table.CurrentIndex() >= 0)
}

func (app *application) beginBusy(detail string) {
	app.busy = true
	app.setDetail(detail)
	app.updateControls()
}

func (app *application) endBusy() {
	app.busy = false
	app.updateControls()
}

func (app *application) cancelChecks() {
	if app.checkCancel != nil {
		app.checkCancel()
		app.checkCancel = nil
	}
}

func (app *application) showSelectedDetail() {
	row, exists := app.model.item(app.table.CurrentIndex())
	if !exists || row.Detail == "" {
		app.updateControls()
		return
	}
	app.setDetail(fmt.Sprintf("%s：%s", row.Proxy, oneLine(row.Detail)))
	app.updateControls()
}

func (app *application) setDetail(message string) {
	_ = app.detailLabel.SetText(oneLine(message))
}

func (app *application) showError(message string) {
	app.setDetail(message)
	walk.MsgBox(app.window, applicationName, message, walk.MsgBoxIconError)
}

func (app *application) styleCell(style *walk.CellStyle) {
	row, exists := app.model.item(style.Row())
	if !exists {
		return
	}
	if style.Col() != 0 {
		return
	}
	switch row.Status {
	case statusNormal:
		style.TextColor = walk.RGB(20, 125, 70)
	case statusCheckFailed, statusConflict:
		style.TextColor = walk.RGB(190, 55, 45)
	case statusChecking, statusStarting:
		style.TextColor = walk.RGB(25, 100, 175)
	default:
		style.TextColor = walk.RGB(85, 85, 85)
	}
}

func (app *application) onClosing(canceled *bool, _ walk.CloseReason) {
	if app.closing {
		return
	}
	app.closing = true
	app.cancelChecks()
	app.applicationCancel()
	if app.service.IsRunning() {
		stopContext, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		_ = app.service.Stop(stopContext)
		cancel()
	}
}

func (app *application) synchronize(callback func()) {
	if app.closing || app.window == nil {
		return
	}
	app.window.Synchronize(func() {
		if !app.closing {
			callback()
		}
	})
}

func recommendStartPort(current, count int) int {
	if count <= 0 || count > 65535 {
		return 0
	}
	start := ((current/100)+1)*100 + 1
	for candidate := start; candidate+count-1 <= 65535; candidate += 100 {
		listeners := make([]net.Listener, 0, count)
		available := true
		for port := candidate; port < candidate+count; port++ {
			listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				available = false
				break
			}
			listeners = append(listeners, listener)
		}
		for _, listener := range listeners {
			_ = listener.Close()
		}
		if available {
			return candidate
		}
	}
	return 0
}

func oneLine(message string) string {
	message = strings.ReplaceAll(message, "\r", " ")
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.Join(strings.Fields(message), " ")
	runes := []rune(message)
	if len(runes) > 240 {
		return string(runes[:240]) + "..."
	}
	return message
}

func defaultImportDirectory() string {
	executablePath, _ := os.Executable()
	return filepath.Dir(executablePath)
}
