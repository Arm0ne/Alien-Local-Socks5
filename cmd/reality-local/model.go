package main

import (
	"sync"

	"github.com/lxn/walk"
)

type portStatus string

const (
	statusReady       portStatus = "未启动"
	statusStarting    portStatus = "正在启动"
	statusListening   portStatus = "已监听"
	statusChecking    portStatus = "正在检测"
	statusNormal      portStatus = "正常"
	statusVerified    portStatus = "检测通过（未启动）"
	statusCheckFailed portStatus = "检测失败"
	statusConflict    portStatus = "端口冲突"
	statusStopped     portStatus = "已停止"
)

type portRow struct {
	Status    portStatus
	Proxy     string
	ExitIP    string
	CheckedAt string
	Detail    string
	Port      int
}

type portTableModel struct {
	walk.TableModelBase
	mu   sync.RWMutex
	rows []portRow
}

func (model *portTableModel) RowCount() int {
	model.mu.RLock()
	defer model.mu.RUnlock()
	return len(model.rows)
}

func (model *portTableModel) Value(row, column int) interface{} {
	model.mu.RLock()
	defer model.mu.RUnlock()
	item := model.rows[row]
	switch column {
	case 0:
		return string(item.Status)
	case 1:
		return item.Proxy
	case 2:
		return item.ExitIP
	case 3:
		return item.CheckedAt
	default:
		return ""
	}
}

func (model *portTableModel) reset(rows []portRow) {
	model.mu.Lock()
	model.rows = append([]portRow(nil), rows...)
	model.mu.Unlock()
	model.PublishRowsReset()
	if len(rows) > 0 {
		// Walk does not reliably repaint a virtual table when a reset keeps the
		// same row count, so explicitly invalidate the replaced row range.
		model.PublishRowsChanged(0, len(rows)-1)
	}
}

func (model *portTableModel) update(index int, update func(*portRow)) {
	model.mu.Lock()
	if index < 0 || index >= len(model.rows) {
		model.mu.Unlock()
		return
	}
	update(&model.rows[index])
	model.mu.Unlock()
	model.PublishRowChanged(index)
}

func (model *portTableModel) snapshot() []portRow {
	model.mu.RLock()
	defer model.mu.RUnlock()
	return append([]portRow(nil), model.rows...)
}

func (model *portTableModel) item(index int) (portRow, bool) {
	model.mu.RLock()
	defer model.mu.RUnlock()
	if index < 0 || index >= len(model.rows) {
		return portRow{}, false
	}
	return model.rows[index], true
}
