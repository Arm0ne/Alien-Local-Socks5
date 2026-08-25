package converter

import (
	"fmt"
	"strings"
)

const DefaultStartPort = 21001

type Node struct {
	SourceLine  int
	Name        string
	Server      string
	ServerPort  int
	UUID        string
	Network     string
	Encryption  string
	Security    string
	PublicKey   string
	Fingerprint string
	ServerName  string
	ShortID     string
	SpiderX     string
	Flow        string
	RawLink     string
}

type Mapping struct {
	Index       int
	InboundTag  string
	OutboundTag string
	ListenPort  int
	NodeName    string
}

type ParseResult struct {
	Nodes    []Node
	Mappings []Mapping
}

type ValidationError struct {
	Line    int
	Message string
}

type ValidationErrors []ValidationError

func (errs ValidationErrors) Error() string {
	lines := make([]string, 0, len(errs))
	for _, item := range errs {
		if item.Line > 0 {
			lines = append(lines, fmt.Sprintf("第 %d 行：%s", item.Line, item.Message))
			continue
		}
		lines = append(lines, item.Message)
	}
	return strings.Join(lines, "\r\n")
}

func indexedTag(prefix string, index int) string {
	return fmt.Sprintf("%s-%02d", prefix, index)
}
