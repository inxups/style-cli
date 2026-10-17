package compile

import "fmt"

// ValidateFail rejects unsupported policies even for constructed indexes.
func ValidateFail(entry Entry, point string, format int) error {
	if entry.Fail == "" {
		return nil
	}
	if entry.Fail != "closed" || format < FailClosedFormat || entry.Target == "system" || (point != "tool.pre" && point != "prompt.submitted" && point != "skill.explicit") {
		return fmt.Errorf("compile: invalid fail-closed policy (requires index format 7 and a blocking point)")
	}
	return nil
}
