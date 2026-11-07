package alter

import (
	"slices"
	"strings"
	"unicode"
)

const (
	managedNixAndroidOutput = "import ./nix/shells/go-ebitengine-android.nix { inherit pkgs ; }"
	managedNixBlenderOutput = "( import ./nix/lib/blender.nix { inherit pkgs ; } ) .extension"
)

type managedNixInspection struct {
	packages managedSetupInspection
	hooks    managedSetupInspection
	android  managedSetupInspection
	blender  managedSetupInspection
}

func inspectManagedNix(content []byte) managedNixInspection {
	unknown := unknownManagedSetup("Nix composition is outside the supported literal forms")
	result := managedNixInspection{unknown, unknown, unknown, unknown}
	tokens, ok := managedNixTokens(string(content))
	if !ok {
		return result
	}
	tokens = managedNixOutputs(tokens)
	if tokens == nil {
		return result
	}
	shellScope := managedNixScope(tokens, "devShells")
	outputs, ok := managedNixOutputAttrs(shellScope)
	if ok {
		shell := outputs["default"]
		if len(shell) >= 3 && shell[0] == "pkgs.mkShell" && shell[1] == "{" && managedNixEnd(shell, 1) == len(shell)-1 {
			attrs, ok := managedNixAttrs(shell[2 : len(shell)-1])
			if ok {
				result.packages = inspectManagedNixPackages(attrs["packages"])
				result.hooks = inspectManagedNixHook(attrs["shellHook"])
			}
		}
	}
	result.android = inspectManagedNixOutput(tokens, "devShells", "android", managedNixAndroidOutput)
	result.blender = inspectManagedNixOutput(tokens, "packages", "blender-agent-bridge-extension", managedNixBlenderOutput)
	return result
}

func inspectManagedNixPackages(tokens []string) managedSetupInspection {
	unknown := unknownManagedSetup("the package expression or its pkgs/Go overrides need manual inspection")
	if len(tokens) == 0 {
		return managedSetupInspection{state: managedSetupMissing, cause: "no packages attribute"}
	}
	if len(tokens) >= 3 && slices.Equal(tokens[:3], []string{"with", "pkgs", ";"}) {
		tokens = tokens[3:]
	}
	loader := "import ./nix/loader.nix { inherit pkgs ; }"
	if strings.Join(tokens, " ") == loader {
		return managedSetupInspection{state: managedSetupPresent}
	}
	if len(tokens) == 0 || tokens[0] != "[" {
		return unknown
	}
	end := managedNixEnd(tokens, 0)
	if end < 0 {
		return unknown
	}
	if end == len(tokens)-1 {
		return managedSetupInspection{state: managedSetupMissing}
	}
	tail := tokens[end+1:]
	for len(tail) > 0 {
		if tail[0] != "++" {
			return unknown
		}
		tail = tail[1:]
		if strings.Join(tail, " ") == loader || strings.Join(tail, " ") == "( "+loader+" )" {
			return managedSetupInspection{state: managedSetupPresent}
		}
		if len(tail) == 0 || tail[0] != "(" && tail[0] != "[" {
			return unknown
		}
		end = managedNixEnd(tail, 0)
		if end < 0 {
			return unknown
		}
		tail = tail[end+1:]
	}
	return unknown
}

func inspectManagedNixHook(tokens []string) managedSetupInspection {
	if len(tokens) == 0 {
		return managedSetupInspection{state: managedSetupMissing}
	}
	if len(tokens) == 1 && strings.HasPrefix(tokens[0], "\x00") && !strings.Contains(tokens[0], "${}") {
		return managedSetupInspection{state: managedSetupMissing, cause: "literal custom hook"}
	}
	hook := "import ./nix/hooks.nix { inherit pkgs ; }"
	expression := strings.Join(tokens, " ")
	if expression == hook || expression == "( "+hook+" )" || strings.HasPrefix(expression, "( "+hook+" ) + ") {
		return managedSetupInspection{state: managedSetupPresent}
	}
	return unknownManagedSetup("shellHook composition or its pkgs/Go overrides need manual inspection, with custom hooks last")
}

func inspectManagedNixOutput(tokens []string, scope, name, expected string) managedSetupInspection {
	attrs, ok := managedNixOutputAttrs(managedNixScope(tokens, scope))
	if !ok {
		return unknownManagedSetup("the per-system " + scope + " composition needs manual inspection for " + name)
	}
	value, exists := attrs[name]
	if !exists {
		return managedSetupInspection{state: managedSetupMissing}
	}
	if strings.Join(value, " ") == expected {
		return managedSetupInspection{state: managedSetupPresent}
	}
	return unknownManagedSetup("the existing " + name + " declaration needs manual verification, not another attribute")
}

func managedNixOutputs(tokens []string) []string {
	if len(tokens) < 2 || tokens[0] != "{" || managedNixEnd(tokens, 0) != len(tokens)-1 {
		return nil
	}
	tokens = tokens[1 : len(tokens)-1]
	var outputs []string
	for len(tokens) > 0 {
		if len(tokens) < 3 || tokens[1] != "=" || strings.ContainsAny(tokens[0], "$\x00") {
			return nil
		}
		name := tokens[0]
		tokens = tokens[2:]
		if name == "outputs" {
			if outputs != nil || len(tokens) == 0 || tokens[0] != "{" {
				return nil
			}
			end := managedNixEnd(tokens, 0)
			if end < 0 || end+1 >= len(tokens) || tokens[end+1] != ":" {
				return nil
			}
			tokens = managedNixLetBody(tokens[end+2:])
		}
		value, end, ok := managedNixValue(tokens, 0)
		if !ok {
			return nil
		}
		if name == "outputs" {
			if len(value) < 2 || value[0] != "{" || managedNixEnd(value, 0) != len(value)-1 {
				return nil
			}
			outputs = value[1 : len(value)-1]
		}
		tokens = tokens[end+1:]
	}
	return outputs
}

func managedNixLetBody(tokens []string) []string {
	if len(tokens) == 0 || tokens[0] != "let" {
		return tokens
	}
	for i := 1; i < len(tokens); i++ {
		switch tokens[i] {
		case "{", "[", "(":
			i = managedNixEnd(tokens, i)
			if i < 0 {
				return nil
			}
		case "let":
			return nil
		case "in":
			return tokens[i+1:]
		}
	}
	return nil
}

func managedNixScope(tokens []string, scope string) []string {
	var selected []string
	for len(tokens) > 0 {
		if len(tokens) < 3 || tokens[1] != "=" || strings.ContainsAny(tokens[0], "$\x00") {
			return nil
		}
		name := tokens[0]
		value, end, ok := managedNixValue(tokens, 2)
		if !ok || len(value) == 0 {
			return nil
		}
		tokens = tokens[end+1:]
		if name != scope && name != scope+".x86_64-linux" {
			continue
		}
		if name == scope && value[0] != "forAllSystems" {
			return nil
		}
		if selected != nil {
			return nil
		}
		if name == scope {
			if len(value) < 6 || value[1] != "(" || value[2] != "system" || value[3] != ":" || managedNixEnd(value, 1) != len(value)-1 {
				return nil
			}
			value = managedNixLetBody(value[4 : len(value)-1])
			if value == nil {
				return nil
			}
		}
		selected = value
	}
	return selected
}

func managedNixOutputAttrs(tokens []string) (map[string][]string, bool) {
	if len(tokens) == 0 {
		return nil, false
	}
	attrs := make(map[string][]string)
	for len(tokens) > 0 {
		var predicate []string
		if tokens[0] == "pkgs.lib.optionalAttrs" {
			if len(tokens) < 2 || tokens[1] != "(" {
				return nil, false
			}
			end := managedNixEnd(tokens, 1)
			if end < 0 {
				return nil, false
			}
			predicate = tokens[2:end]
			if len(predicate) == 0 {
				return nil, false
			}
			tokens = tokens[end+1:]
		}
		if len(tokens) == 0 || tokens[0] != "{" {
			return nil, false
		}
		end := managedNixEnd(tokens, 0)
		if end < 0 {
			return nil, false
		}
		set, ok := managedNixAttrs(tokens[1:end])
		if !ok {
			return nil, false
		}
		if predicate != nil {
			switch managedNixOptionalAttrs(predicate, set) {
			case managedSetupMissing:
				set = nil
			case managedSetupUnknown:
				return nil, false
			}
		}
		for name, value := range set {
			if _, exists := attrs[name]; exists {
				return nil, false
			}
			attrs[name] = value
		}
		tokens = tokens[end+1:]
		if len(tokens) == 0 {
			break
		}
		if tokens[0] != "//" || len(tokens) == 1 {
			return nil, false
		}
		tokens = tokens[1:]
	}
	return attrs, true
}

// Generated guards establish adoption only for the output that imports the guarded path.
// They do not establish path availability or runtime compatibility.
func managedNixOptionalAttrs(predicate []string, attrs map[string][]string) managedSetupState {
	switch strings.Join(predicate, " ") {
	case "true":
		return managedSetupPresent
	case "false":
		return managedSetupMissing
	case "builtins.pathExists ./nix/lib/blender.nix":
		if len(attrs) == 1 && strings.Join(attrs["blender-agent-bridge-extension"], " ") == managedNixBlenderOutput {
			return managedSetupPresent
		}
	case "system = = \x00x86_64-linux && builtins.pathExists ./nix/shells/go-ebitengine-android.nix":
		if len(attrs) == 1 && strings.Join(attrs["android"], " ") == managedNixAndroidOutput {
			return managedSetupPresent
		}
	}
	return managedSetupUnknown
}

func managedNixAttrs(tokens []string) (map[string][]string, bool) {
	attrs := make(map[string][]string)
	for len(tokens) > 0 {
		if len(tokens) < 3 || tokens[1] != "=" || strings.ContainsAny(tokens[0], ".$\x00") {
			return nil, false
		}
		if _, exists := attrs[tokens[0]]; exists {
			return nil, false
		}
		value, end, ok := managedNixValue(tokens, 2)
		if !ok {
			return nil, false
		}
		attrs[tokens[0]] = value
		tokens = tokens[end+1:]
	}
	return attrs, true
}

func managedNixValue(tokens []string, start int) ([]string, int, bool) {
	for i := start; i < len(tokens); i++ {
		switch tokens[i] {
		case "{", "(", "[":
			end := managedNixEnd(tokens, i)
			if end < 0 {
				return nil, 0, false
			}
			i = end
		case "let", "inherit":
			return nil, 0, false
		case ";":
			if i == start+2 && tokens[start] == "with" {
				continue
			}
			return tokens[start:i], i, true
		}
	}
	return nil, 0, false
}

func managedNixEnd(tokens []string, start int) int {
	pairs := map[string]string{"{": "}", "[": "]", "(": ")"}
	stack := []string{pairs[tokens[start]]}
	for i := start + 1; i < len(tokens); i++ {
		if close, ok := pairs[tokens[i]]; ok {
			stack = append(stack, close)
			continue
		}
		if tokens[i] != "}" && tokens[i] != "]" && tokens[i] != ")" {
			continue
		}
		if len(stack) == 0 || tokens[i] != stack[len(stack)-1] {
			return -1
		}
		stack = stack[:len(stack)-1]
		if len(stack) == 0 {
			return i
		}
	}
	return -1
}

// Strings are opaque tokens, except the platform literal in the generated Android guard.
// Interpolation is marked separately from literal content.
func managedNixTokens(source string) ([]string, bool) {
	var tokens []string
	for i := 0; i < len(source); {
		if unicode.IsSpace(rune(source[i])) {
			i++
			continue
		}
		if source[i] == '#' {
			for i < len(source) && source[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(source[i:], "/*") {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return nil, false
			}
			i += end + 4
			continue
		}
		if source[i] == '"' || strings.HasPrefix(source[i:], "''") {
			end, interpolated, ok := managedNixStringEnd(source, i, 0)
			if !ok {
				return nil, false
			}
			literal := "\x00"
			if interpolated {
				literal = "\x00${}"
			} else if source[i:end] == `"x86_64-linux"` {
				literal = "\x00x86_64-linux"
			}
			tokens = append(tokens, literal)
			i = end
			continue
		}
		if strings.HasPrefix(source[i:], "${") {
			tokens = append(tokens, "{")
			i += 2
			continue
		}
		if strings.ContainsRune("{}[]();=:+", rune(source[i])) {
			token := source[i : i+1]
			if i+1 < len(source) && source[i] == '+' && source[i+1] == '+' {
				token = "++"
			}
			tokens = append(tokens, token)
			i += len(token)
			continue
		}
		start := i
		for i < len(source) && !unicode.IsSpace(rune(source[i])) && !strings.ContainsRune("{}[]();=:+\"#", rune(source[i])) {
			if strings.HasPrefix(source[i:], "${") {
				break
			}
			i++
		}
		if i == start {
			return nil, false
		}
		tokens = append(tokens, source[start:i])
	}
	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "{", "[", "(":
			end := managedNixEnd(tokens, i)
			if end < 0 {
				return nil, false
			}
			i = end
		case "}", "]", ")":
			return nil, false
		}
	}
	return tokens, true
}

func managedNixStringEnd(source string, start, depth int) (end int, interpolated, ok bool) {
	if depth > 64 {
		return 0, false, false
	}
	delimiter := "\""
	if strings.HasPrefix(source[start:], "''") {
		delimiter = "''"
	}
	for i := start + len(delimiter); i < len(source); {
		if delimiter == "\"" && source[i] == '\\' {
			i += 2
			continue
		}
		if delimiter == "''" && (strings.HasPrefix(source[i:], "''${") || strings.HasPrefix(source[i:], "'''") || strings.HasPrefix(source[i:], "''\\")) {
			i += 3
			continue
		}
		if strings.HasPrefix(source[i:], delimiter) {
			return i + len(delimiter), interpolated, true
		}
		if strings.HasPrefix(source[i:], "${") {
			interpolated = true
			braces := 1
			i += 2
			for i < len(source) && braces > 0 {
				if source[i] == '"' || strings.HasPrefix(source[i:], "''") {
					end, _, ok := managedNixStringEnd(source, i, depth+1)
					if !ok {
						return 0, false, false
					}
					i = end
					continue
				}
				if source[i] == '#' || strings.HasPrefix(source[i:], "/*") {
					return 0, false, false
				}
				if source[i] == '{' {
					braces++
				}
				if source[i] == '}' {
					braces--
				}
				i++
			}
			if braces != 0 {
				return 0, false, false
			}
			continue
		}
		i++
	}
	return 0, false, false
}
