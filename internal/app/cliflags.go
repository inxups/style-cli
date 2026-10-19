package app

import (
	"fmt"
	"io"
	"strings"
)

type flagKind int

const (
	flagBool   flagKind = iota // --x only; never takes or consumes a value
	flagString                 // --x v or --x=v; at most once
	flagList                   // --x v or --x=v; repeatable
)

type flagGroup int

const (
	groupMain flagGroup = iota
	groupAdvanced
	groupProject
)

type flagSpec struct {
	Name     string
	Kind     flagKind
	Value    string // usage placeholder for string and list flags
	Group    flagGroup
	Required bool
	Hidden   bool // documented by the positional usage instead
}

// commandSpec is the single source for a command's parser and its usage text.
type commandSpec struct {
	Name    string
	Args    string // positional usage, e.g. "<source>" or "[name]"
	MaxArgs int
	Flags   []flagSpec
	Project bool // accepts the global --project scope
}

type parsedArgs struct {
	Positional []string
	values     map[string][]string
}

func (p parsedArgs) Has(name string) bool      { _, ok := p.values[name]; return ok }
func (p parsedArgs) List(name string) []string { return p.values[name] }
func (p parsedArgs) Bool(name string) bool     { return p.String(name) == "true" }
func (p parsedArgs) Arg(index int) string {
	if index < len(p.Positional) {
		return p.Positional[index]
	}
	return ""
}
func (p parsedArgs) String(name string) string {
	if values := p.values[name]; len(values) > 0 {
		return values[len(values)-1]
	}
	return ""
}

func helpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func (c commandSpec) flag(name string) (flagSpec, bool) {
	for _, flag := range c.Flags {
		if flag.Name == name {
			return flag, true
		}
	}
	return flagSpec{}, false
}

// parse never sees --help; callers check helpRequested first so help works in
// any position, even next to otherwise invalid arguments.
func (c commandSpec) parse(args []string) (parsedArgs, error) {
	parsed := parsedArgs{values: map[string][]string{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			if len(parsed.Positional) >= c.MaxArgs {
				return parsed, fmt.Errorf("unexpected argument %q", arg)
			}
			parsed.Positional = append(parsed.Positional, arg)
			continue
		}
		name, value, inline := strings.Cut(arg, "=")
		flag, ok := c.flag(name)
		if !ok {
			return parsed, fmt.Errorf("unknown option %q", name)
		}
		if flag.Kind != flagList && parsed.Has(name) {
			return parsed, fmt.Errorf("duplicate option %s", name)
		}
		if flag.Kind == flagBool {
			if inline {
				return parsed, fmt.Errorf("%s does not take a value", name)
			}
			parsed.values[name] = []string{"true"}
			continue
		}
		if !inline {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return parsed, fmt.Errorf("missing value for %s", name)
			}
			i++
			value = args[i]
		}
		if value == "" {
			return parsed, fmt.Errorf("empty value for %s", name)
		}
		parsed.values[name] = append(parsed.values[name], value)
	}
	for _, flag := range c.Flags {
		if flag.Required && !parsed.Has(flag.Name) {
			return parsed, fmt.Errorf("missing required option %s", flag.Name)
		}
	}
	return parsed, nil
}

func (f flagSpec) usage() string {
	text := f.Name
	if f.Kind != flagBool {
		text += " " + f.Value
	}
	if f.Required {
		return text
	}
	text = "[" + text + "]"
	if f.Kind == flagList {
		text += "..."
	}
	return text
}

func (c commandSpec) usageLine(group flagGroup) string {
	var parts []string
	for _, flag := range c.Flags {
		if flag.Group == group && !flag.Hidden {
			parts = append(parts, flag.usage())
		}
	}
	return strings.Join(parts, " ")
}

// summary is the main usage line shown in the top-level help.
func (c commandSpec) summary() string {
	main := "style " + c.Name
	if c.Args != "" {
		main += " " + c.Args
	}
	if flags := c.usageLine(groupMain); flags != "" {
		main += " " + flags
	}
	return main
}

// usage is the full per-command help: the main line plus advanced and project groups.
func (c commandSpec) usage() string {
	lines := []string{c.summary()}
	if flags := c.usageLine(groupAdvanced); flags != "" {
		lines = append(lines, "  advanced: "+flags)
	}
	project := c.usageLine(groupProject)
	if c.Project {
		project = strings.TrimSpace(projectFlagsUsage + " " + project)
	}
	if project != "" {
		lines = append(lines, "  project: "+project)
	}
	return strings.Join(lines, "\n")
}

const projectFlagsUsage = "[--project dir] [--style-home dir]"

// usageError prints the error and the command usage to errOut.
func (c commandSpec) usageError(errOut io.Writer, err error) int {
	fmt.Fprintf(errOut, "style %s: %v\n%s\n", c.Name, err, c.usage())
	return ExitUsage
}

var (
	jsonFlag    = flagSpec{Name: "--json", Kind: flagBool}
	allFlag     = flagSpec{Name: "--all", Kind: flagBool, Hidden: true}
	refFlag     = flagSpec{Name: "--ref", Kind: flagString, Value: "ref"}
	replaceFlag = flagSpec{Name: "--allow-replace", Kind: flagBool}
	harnessFlag = flagSpec{Name: "--harness", Kind: flagList, Value: "auto|h[,h]"}
	exactFlag   = flagSpec{Name: "--exact", Kind: flagString, Value: "openai:<encoding>|anthropic:<model>"}
)

var (
	initCommand     = commandSpec{Name: "init", Args: "[name]", MaxArgs: 1, Flags: []flagSpec{{Name: "--single", Kind: flagBool}}}
	validateCommand = commandSpec{Name: "validate", Args: "[source]", MaxArgs: 1, Flags: []flagSpec{jsonFlag}}
	countCommand    = commandSpec{Name: "count", Args: "<UTF-8-file>", MaxArgs: 1, Flags: []flagSpec{exactFlag, jsonFlag}}
	planCommand     = commandSpec{Name: "plan", Args: "[source]", MaxArgs: 1, Project: true, Flags: []flagSpec{
		harnessFlag, exactFlag, jsonFlag,
		{Name: "--assume-requests", Kind: flagString, Value: "N", Group: groupAdvanced},
		{Name: "--assume-triggers", Kind: flagString, Value: "N", Group: groupAdvanced},
	}}
	installCommand = commandSpec{Name: "install", Args: "<source>", MaxArgs: 1, Project: true, Flags: []flagSpec{
		harnessFlag,
		{Name: "--set", Kind: flagList, Value: "k=v"},
		jsonFlag,
		{Name: "--route", Kind: flagList, Value: "h=r", Group: groupAdvanced},
		{Name: "--link", Kind: flagBool, Group: groupAdvanced},
		{Name: "--allow-replace", Kind: flagBool, Group: groupAdvanced},
		{Name: "--fail-closed", Kind: flagBool, Group: groupAdvanced},
		{Name: "--project-source", Kind: flagList, Value: "source", Group: groupProject},
		{Name: "--project-set", Kind: flagList, Value: "package.key=value", Group: groupProject},
	}}
	restoreCommand   = commandSpec{Name: "restore", Project: true, Flags: []flagSpec{replaceFlag, jsonFlag}}
	updateCommand    = commandSpec{Name: "update", Args: "<name|--all>", MaxArgs: 1, Project: true, Flags: []flagSpec{allFlag, refFlag, replaceFlag, jsonFlag}}
	uninstallCommand = commandSpec{Name: "uninstall", Args: "<name|--all>", MaxArgs: 1, Project: true, Flags: []flagSpec{allFlag, jsonFlag}}
	listCommand      = commandSpec{Name: "list", Project: true, Flags: []flagSpec{jsonFlag}}
	syncCommand      = commandSpec{Name: "sync", Project: true, Flags: []flagSpec{jsonFlag}}
	trustCommand     = commandSpec{Name: "trust", Args: "<name>", MaxArgs: 1, Project: true, Flags: []flagSpec{replaceFlag, jsonFlag}}
	doctorCommand    = commandSpec{Name: "doctor", Project: true, Flags: []flagSpec{{Name: "--reset-state", Kind: flagBool}, jsonFlag}}
	statsCommand     = commandSpec{Name: "stats", Project: true, Flags: []flagSpec{{Name: "--by", Kind: flagString, Value: "rule|session"}, {Name: "--since", Kind: flagString, Value: "<duration|RFC3339>"}, jsonFlag}}
	explainCommand   = commandSpec{Name: "explain", Project: true, Flags: []flagSpec{
		{Name: "--harness", Kind: flagList, Value: "h", Required: true},
		{Name: "--payload", Kind: flagString, Value: "f.json", Required: true},
		{Name: "--event", Kind: flagString, Value: "NativeEvent"},
		{Name: "--fail-closed", Kind: flagBool},
		exactFlag, jsonFlag,
		{Name: "--index", Kind: flagString, Value: "f", Group: groupAdvanced},
		{Name: "--state-dir", Kind: flagString, Value: "d", Group: groupAdvanced},
		{Name: "--facts", Kind: flagString, Value: "f", Group: groupAdvanced},
		{Name: "--provenance", Kind: flagString, Value: "f", Group: groupAdvanced},
	}}
	probeCommand = commandSpec{Name: "probe", Project: true, Flags: []flagSpec{harnessFlag, {Name: "--timeout", Kind: flagString, Value: "60s"}, jsonFlag}}
)

// commandSections orders the top-level help.
var commandSections = []struct {
	Title    string
	Commands []commandSpec
}{
	{"Authoring", []commandSpec{initCommand, validateCommand, countCommand, planCommand}},
	{"Installation", []commandSpec{installCommand, restoreCommand, updateCommand, uninstallCommand, syncCommand, trustCommand}},
	{"Diagnostics", []commandSpec{listCommand, doctorCommand, probeCommand, statsCommand, explainCommand}},
}

func lookupCommand(name string) (commandSpec, bool) {
	for _, section := range commandSections {
		for _, command := range section.Commands {
			if command.Name == name {
				return command, true
			}
		}
	}
	return commandSpec{}, false
}

func printUsage(out io.Writer) {
	for i, section := range commandSections {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, section.Title+":")
		for _, command := range section.Commands {
			fmt.Fprintln(out, "  "+command.summary())
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Global:")
	fmt.Fprintln(out, "  "+projectFlagsUsage+" scopes plan, installation and diagnostic commands to a project.")
	fmt.Fprintln(out, "  style help <command> (or <command> --help) shows advanced and project options.")
}
