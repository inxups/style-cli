package install

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

const windowsCommandPrefix = "powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand "
const windowsScriptPrefix = "$ErrorActionPreference='Stop'; & "
const windowsScriptSuffix = "; exit $LASTEXITCODE"

func windowsHookCommand(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, arg := range argv {
		parts = append(parts, "'"+strings.ReplaceAll(arg, "'", "''")+"'")
	}
	units := utf16.Encode([]rune(windowsScriptPrefix + strings.Join(parts, " ") + windowsScriptSuffix))
	data := make([]byte, len(units)*2)
	for i, value := range units {
		binary.LittleEndian.PutUint16(data[2*i:], value)
	}
	return windowsCommandPrefix + base64.StdEncoding.EncodeToString(data)
}

func windowsDecodedCommand(command string) (string, bool) {
	if !strings.HasPrefix(command, windowsCommandPrefix) || len(command) > 32767 {
		return "", false
	}
	data, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(command, windowsCommandPrefix))
	if err != nil || len(data)%2 != 0 {
		return "", false
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	script := string(utf16.Decode(units))
	if !strings.HasPrefix(script, windowsScriptPrefix) || !strings.HasSuffix(script, windowsScriptSuffix) {
		return "", false
	}
	return script, true
}
