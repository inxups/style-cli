package install

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

const windowsCommandOptions = " -NoLogo -NoProfile -NonInteractive -EncodedCommand "

func windowsCommandPrefix() string {
	launcher, err := windowsLauncherPath()
	if err != nil {
		return ""
	}
	return launcher + windowsCommandOptions
}
func safeWindowsLauncher(path string) bool {
	if len(path) < 4 || path[1] != ':' || path[2] != '/' || !strings.HasSuffix(strings.ToLower(path), "/windowspowershell/v1.0/powershell.exe") {
		return false
	}
	if !(path[0] >= 'A' && path[0] <= 'Z' || path[0] >= 'a' && path[0] <= 'z') {
		return false
	}
	for _, char := range path[3:] {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("/_-.", char)) {
			return false
		}
	}
	return !strings.Contains(path, "/../") && !strings.Contains(path, "/./")
}

const windowsScriptPrefix = "$ErrorActionPreference='Stop'; $p=New-Object System.Diagnostics.Process; $p.StartInfo.UseShellExecute=$false; $p.StartInfo.FileName="
const windowsScriptSuffix = "; if(-not $p.Start()){exit 1}; $p.WaitForExit(); exit $p.ExitCode"

// quoteWindowsArgument follows CommandLineToArgvW backslash/quote rules.
func quoteWindowsArgument(arg string) string {
	var out strings.Builder
	out.WriteByte('"')
	slashes := 0
	for _, char := range arg {
		if char == '\\' {
			slashes++
			continue
		}
		if char == '"' {
			out.WriteString(strings.Repeat("\\", slashes*2+1))
		} else {
			out.WriteString(strings.Repeat("\\", slashes))
		}
		slashes = 0
		out.WriteRune(char)
	}
	out.WriteString(strings.Repeat("\\", slashes*2))
	out.WriteByte('"')
	return out.String()
}
func windowsHookCommand(argv []string) string {
	parts := make([]string, 0, len(argv)-1)
	for _, arg := range argv[1:] {
		parts = append(parts, quoteWindowsArgument(arg))
	}
	literal := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	// ProcessStartInfo inherits the original standard handles. No PowerShell
	// native argv reserialization or text pipeline touches the hook protocol.
	script := windowsScriptPrefix + literal(argv[0]) + "; $p.StartInfo.Arguments=" + literal(strings.Join(parts, " ")) + windowsScriptSuffix
	units := utf16.Encode([]rune(script))
	data := make([]byte, len(units)*2)
	for i, value := range units {
		binary.LittleEndian.PutUint16(data[2*i:], value)
	}
	return windowsCommandPrefix() + base64.StdEncoding.EncodeToString(data)
}

func windowsDecodedCommand(command string) (string, bool) {
	launcher, encoded, found := strings.Cut(command, windowsCommandOptions)
	if !found || !safeWindowsLauncher(launcher) || len(command) > 32767 {
		return "", false
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
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
	rest := strings.TrimPrefix(script, windowsScriptPrefix)
	_, rest, ok := windowsScriptLiteral(rest)
	if !ok || !strings.HasPrefix(rest, "; $p.StartInfo.Arguments=") {
		return "", false
	}
	args, rest, ok := windowsScriptLiteral(strings.TrimPrefix(rest, "; $p.StartInfo.Arguments="))
	if !ok || rest != windowsScriptSuffix {
		return "", false
	}
	return args, true
}

func windowsScriptLiteral(text string) (string, string, bool) {
	if !strings.HasPrefix(text, "'") {
		return "", "", false
	}
	var result strings.Builder
	for i := 1; i < len(text); i++ {
		if text[i] != '\'' {
			result.WriteByte(text[i])
			continue
		}
		if i+1 < len(text) && text[i+1] == '\'' {
			result.WriteByte('\'')
			i++
			continue
		}
		return result.String(), text[i+1:], true
	}
	return "", "", false
}
