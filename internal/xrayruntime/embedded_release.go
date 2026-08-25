//go:build integrated_xray

package xrayruntime

import _ "embed"

//go:embed assets/xray.exe
var embeddedXray []byte
