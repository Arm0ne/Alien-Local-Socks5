package main

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"realityconverter/internal/appcore"
	"realityconverter/internal/profile"
)

func TestRecommendStartPort(t *testing.T) {
	listeners := make([]net.Listener, 0, 2)
	for _, port := range []int{21101, 21102} {
		listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
		if err != nil {
			t.Skipf("test port is unavailable: %v", err)
		}
		listeners = append(listeners, listener)
	}
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	if recommendation := recommendStartPort(21001, 2); recommendation != 21201 {
		t.Fatalf("recommendation = %d, want 21201", recommendation)
	}
}

func TestExpiryDescriptionKeepsExpiredTimeAndWarning(t *testing.T) {
	expired := time.Now().Add(-time.Hour)
	text := expiryDescription(&expired, "")
	if !strings.Contains(text, "到期 ") || !strings.Contains(text, "已过期，仅提示，仍可启动") {
		t.Fatalf("expiry description = %q", text)
	}

	future := time.Now().Add(time.Hour)
	text = expiryDescription(&future, "")
	if strings.Contains(text, "已过期") || !strings.Contains(text, "到期 ") {
		t.Fatalf("future expiry description = %q", text)
	}
}

func TestSourceDescriptionIncludesFullFilePath(t *testing.T) {
	path := `C:\Users\Alien\Desktop\nodes\reality-list.txt`
	got := sourceDescription(appcore.SourceInfo{Kind: profile.SourceKindFile, FilePath: path})
	if !strings.Contains(got, path) {
		t.Fatalf("source description = %q, want full path", got)
	}
	legacy := sourceDescription(appcore.SourceInfo{Kind: profile.SourceKindFile})
	if !strings.Contains(legacy, "未记录路径") {
		t.Fatalf("legacy source description = %q", legacy)
	}
}

func TestSummaryCountsOfflineVerificationWithoutListener(t *testing.T) {
	rows := []portRow{
		{Status: statusVerified},
		{Status: statusCheckFailed},
		{Status: statusReady},
	}
	listening, normal, failed := summaryCounts(rows, false)
	if listening != 0 || normal != 1 || failed != 1 {
		t.Fatalf("offline summary = %d, %d, %d", listening, normal, failed)
	}
	listening, normal, failed = summaryCounts(rows, true)
	if listening != 1 || normal != 1 || failed != 1 {
		t.Fatalf("running summary = %d, %d, %d", listening, normal, failed)
	}
}
