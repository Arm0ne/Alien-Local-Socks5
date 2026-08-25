package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"realityconverter/internal/converter"
)

var version = "dev"

type application struct {
	window        *walk.MainWindow
	inputPath     *walk.LineEdit
	inputBrowse   *walk.PushButton
	outputPath    *walk.LineEdit
	outputBrowse  *walk.PushButton
	startPort     *walk.NumberEdit
	convertButton *walk.PushButton
	resultText    *walk.TextEdit
	statusLabel   *walk.Label
}

func main() {
	app := &application{}
	if err := app.run(); err != nil {
		walk.MsgBox(nil, "Reality 转换器", err.Error(), walk.MsgBoxIconError)
		os.Exit(1)
	}
}

func (app *application) run() error {
	mainWindow := MainWindow{
		AssignTo: &app.window,
		Title:    "Reality 链接转换器",
		Size:     Size{Width: 780, Height: 590},
		MinSize:  Size{Width: 680, Height: 500},
		Font:     Font{Family: "Microsoft YaHei UI", PointSize: 9},
		Layout:   VBox{Margins: Margins{Left: 16, Top: 14, Right: 16, Bottom: 14}, Spacing: 10},
		Children: []Widget{
			Label{
				Text: "VLESS + TCP + Reality 转换为本地独立 SOCKS5 端口",
				Font: Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true},
			},
			Composite{
				Layout: Grid{Columns: 3, MarginsZero: true, Spacing: 8},
				Children: []Widget{
					Label{Text: "节点 TXT", MinSize: Size{Width: 82}},
					LineEdit{
						AssignTo:  &app.inputPath,
						CueBanner: "选择每行一个 vless:// 链接的 TXT 文件",
					},
					PushButton{
						AssignTo:  &app.inputBrowse,
						Text:      "选择...",
						MinSize:   Size{Width: 88},
						OnClicked: app.chooseInput,
					},
					Label{Text: "输出 JSON"},
					LineEdit{
						AssignTo:  &app.outputPath,
						CueBanner: "选择 config.json；同目录必须有 xray.exe",
					},
					PushButton{
						AssignTo:  &app.outputBrowse,
						Text:      "选择...",
						MinSize:   Size{Width: 88},
						OnClicked: app.chooseOutput,
					},
					Label{Text: "起始端口"},
					NumberEdit{
						AssignTo:           &app.startPort,
						MinValue:           1,
						MaxValue:           65535,
						Decimals:           0,
						Increment:          1,
						SpinButtonsVisible: true,
					},
					HSpacer{},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 10},
				Children: []Widget{
					PushButton{
						AssignTo:  &app.convertButton,
						Text:      "转换并检查",
						MinSize:   Size{Width: 128, Height: 34},
						OnClicked: app.convert,
					},
					Label{
						AssignTo: &app.statusLabel,
						Text:     "等待转换",
					},
					HSpacer{},
					Label{Text: "版本 " + version, TextColor: walk.RGB(100, 100, 100)},
				},
			},
			Label{Text: "结果"},
			TextEdit{
				AssignTo:      &app.resultText,
				ReadOnly:      true,
				VScroll:       true,
				HScroll:       true,
				MinSize:       Size{Height: 270},
				StretchFactor: 1,
				Text:          "转换成功后将在这里显示端口映射。",
			},
		},
	}

	if err := mainWindow.Create(); err != nil {
		return err
	}
	if err := app.startPort.SetValue(converter.DefaultStartPort); err != nil {
		app.window.Dispose()
		return err
	}
	app.window.Run()
	return nil
}

func (app *application) chooseInput() {
	dialog := &walk.FileDialog{
		Title:  "选择节点 TXT",
		Filter: "文本文件 (*.txt)|*.txt|所有文件 (*.*)|*.*",
	}
	if current := strings.TrimSpace(app.inputPath.Text()); current != "" {
		dialog.FilePath = current
	}
	accepted, err := dialog.ShowOpen(app.window)
	if err != nil {
		app.showError("打开文件选择窗口失败：" + err.Error())
		return
	}
	if !accepted {
		return
	}
	_ = app.inputPath.SetText(dialog.FilePath)
	if strings.TrimSpace(app.outputPath.Text()) == "" {
		_ = app.outputPath.SetText(filepath.Join(filepath.Dir(dialog.FilePath), "config.json"))
	}
}

func (app *application) chooseOutput() {
	defaultPath := strings.TrimSpace(app.outputPath.Text())
	if defaultPath == "" {
		if input := strings.TrimSpace(app.inputPath.Text()); input != "" {
			defaultPath = filepath.Join(filepath.Dir(input), "config.json")
		} else {
			defaultPath = "config.json"
		}
	}
	dialog := &walk.FileDialog{
		Title:    "选择输出 JSON",
		Filter:   "JSON 配置 (*.json)|*.json|所有文件 (*.*)|*.*",
		FilePath: defaultPath,
	}
	accepted, err := dialog.ShowSave(app.window)
	if err != nil {
		app.showError("打开文件选择窗口失败：" + err.Error())
		return
	}
	if !accepted {
		return
	}
	path := dialog.FilePath
	if filepath.Ext(path) == "" {
		path += ".json"
	}
	_ = app.outputPath.SetText(path)
}

func (app *application) convert() {
	inputPath := strings.TrimSpace(app.inputPath.Text())
	outputPath := strings.TrimSpace(app.outputPath.Text())
	startPort := int(app.startPort.Value())

	if inputPath == "" {
		app.showError("请选择输入 TXT 文件。")
		return
	}
	if outputPath == "" {
		app.showError("请选择输出 JSON 文件。")
		return
	}

	app.setBusy(true)
	_ = app.resultText.SetText("正在解析节点并执行 Xray 配置检查...")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		result, err := converter.ConvertFile(ctx, converter.ConvertOptions{
			InputPath:  inputPath,
			OutputPath: outputPath,
			StartPort:  startPort,
			Timeout:    30 * time.Second,
		}, converter.ProcessChecker{})

		app.window.Synchronize(func() {
			app.setBusy(false)
			if err != nil {
				app.showError(err.Error())
				return
			}
			_ = app.statusLabel.SetText("转换成功，Xray 配置检查通过")
			_ = app.resultText.SetText(formatSuccess(result, outputPath))
		})
	}()
}

func (app *application) setBusy(busy bool) {
	app.convertButton.SetEnabled(!busy)
	app.inputPath.SetEnabled(!busy)
	app.inputBrowse.SetEnabled(!busy)
	app.outputPath.SetEnabled(!busy)
	app.outputBrowse.SetEnabled(!busy)
	app.startPort.SetEnabled(!busy)
	if busy {
		_ = app.statusLabel.SetText("正在转换...")
	}
}

func (app *application) showError(message string) {
	_ = app.statusLabel.SetText("转换失败")
	_ = app.resultText.SetText(message)
}

func formatSuccess(result converter.ParseResult, outputPath string) string {
	var output strings.Builder
	fmt.Fprintf(&output, "Converted %d nodes.\r\n\r\n", len(result.Nodes))
	for _, mapping := range result.Mappings {
		fmt.Fprintf(&output, "127.0.0.1:%d -> %s", mapping.ListenPort, mapping.OutboundTag)
		if mapping.NodeName != "" {
			fmt.Fprintf(&output, "  (%s)", singleLine(mapping.NodeName))
		}
		output.WriteString("\r\n")
	}
	fmt.Fprintf(&output, "\r\n配置：%s\r\nXray 配置检查：通过", outputPath)
	return output.String()
}

func singleLine(value string) string {
	value = strings.Map(func(character rune) rune {
		if character < 32 || character == 127 {
			return ' '
		}
		return character
	}, value)
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 80 {
		return string(runes[:80]) + "..."
	}
	return value
}
