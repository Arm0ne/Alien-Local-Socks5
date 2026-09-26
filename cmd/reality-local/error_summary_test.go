package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"realityconverter/internal/converter"
)

func TestSubscriptionErrorMessageSummarizesValidationErrors(t *testing.T) {
	for _, count := range []int{1, 20, 1000} {
		t.Run(fmt.Sprintf("errors_%d", count), func(t *testing.T) {
			validationErrors := make(converter.ValidationErrors, count)
			for i := range validationErrors {
				validationErrors[i] = converter.ValidationError{
					Line:    i + 1,
					Message: fmt.Sprintf("错误编号 %d", i+1),
				}
			}

			message := subscriptionErrorMessage(fmt.Errorf("订阅节点解析失败：%w", validationErrors))
			visible := count
			if visible > 4 {
				visible = 4
			}
			for line := 1; line <= visible; line++ {
				want := fmt.Sprintf("第 %d 行：错误编号 %d", line, line)
				if !strings.Contains(message, want) {
					t.Errorf("summary missing %q: %q", want, message)
				}
			}
			if count <= 4 {
				if strings.Contains(message, "省略") || strings.Contains(message, "其余") {
					t.Errorf("single error should not indicate omitted errors: %q", message)
				}
				return
			}
			if strings.Contains(message, "第 5 行：") {
				t.Errorf("summary displays more than four specific errors: %q", message)
			}
			if !strings.Contains(message, fmt.Sprint(count)) || !strings.Contains(message, fmt.Sprint(count-4)) {
				t.Errorf("summary should include total %d and omitted %d: %q", count, count-4, message)
			}
		})
	}
}

func TestSubscriptionErrorMessageLeavesOtherErrorsUnchanged(t *testing.T) {
	message := "订阅请求失败：HTTP 403\r\n请检查地址"
	if got := subscriptionErrorMessage(errors.New(message)); got != message {
		t.Fatalf("message = %q, want %q", got, message)
	}
}

func TestSubscriptionErrorMessageBoundsLongReasonAndHidesLink(t *testing.T) {
	secretLink := "vless://11111111-1111-1111-1111-111111111111@example.com:443?pbk=secret"
	issues := converter.ValidationErrors{
		{Line: 1, Message: "链接格式无效：" + secretLink},
		{Line: 2, Message: strings.Repeat("格式错误", 100)},
	}
	message := subscriptionErrorMessage(issues)
	if strings.Contains(message, secretLink) || !strings.Contains(message, "第 1 行：链接格式无效（原始链接已隐藏）") {
		t.Fatalf("link not hidden in %q", message)
	}
	for _, line := range strings.Split(message, "\r\n") {
		if utf8.RuneCountInString(line) > 90 {
			t.Fatalf("dialog line too long: %q", line)
		}
	}
}

func TestStatusLineIsSingleLineAndLimitedTo48Runes(t *testing.T) {
	short := "第 1 行：仅支持 TCP，收到 ws"
	if got := statusLine(short); got != short {
		t.Fatalf("short status = %q, want %q", got, short)
	}

	long := strings.Repeat("节点参数错误", 20) + "\r\n第 2 行：协议不是 VLESS"
	got := statusLine(long)
	if utf8.RuneCountInString(got) > 48 {
		t.Errorf("status contains %d runes, want at most 48: %q", utf8.RuneCountInString(got), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncated status should end with ellipsis: %q", got)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("status should not contain line breaks: %q", got)
	}
}

func TestStatusLineFlattensLineBreaksWithoutTruncatingShortMessage(t *testing.T) {
	got := statusLine("错误一\r\n错误二\n错误三")
	if got != "错误一 错误二 错误三" {
		t.Fatalf("status = %q, want flattened error text", got)
	}
}
