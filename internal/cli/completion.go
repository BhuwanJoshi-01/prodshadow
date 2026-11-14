package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/BhuwanJoshi-01/prodshadow/schemas"
)

// completionFlag is one flag of a command, as shown by its -h output.
type completionFlag struct {
	name, help string
}

func runCompletion(ctx context.Context, env *Env, args []string) int {
	const synopsis = "Usage: prodshadow completion bash|zsh|fish\n\n" +
		"Prints a shell completion script. For example:\n" +
		"  bash: source <(prodshadow completion bash)\n" +
		"  zsh:  source <(prodshadow completion zsh)\n" +
		"  fish: prodshadow completion fish > ~/.config/fish/completions/prodshadow.fish\n"
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprint(env.Stderr, synopsis)
		if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			return ExitOK
		}
		return ExitInvalid
	}
	flags := commandFlags(ctx)
	switch args[0] {
	case "bash":
		writeBashCompletion(env.Stdout, flags)
	case "zsh":
		fmt.Fprintln(env.Stdout, "#compdef prodshadow\nautoload -U +X bashcompinit && bashcompinit")
		writeBashCompletion(env.Stdout, flags)
	case "fish":
		writeFishCompletion(env.Stdout, flags)
	default:
		fmt.Fprintf(env.Stderr, "prodshadow: unsupported shell %q\n\n%s", args[0], synopsis)
		return ExitInvalid
	}
	return ExitOK
}

// commandFlags collects each command's flags from its -h output, so the
// completion scripts cannot drift from the real flag sets. Parsing -h
// happens before a command does any work.
func commandFlags(ctx context.Context) map[string][]completionFlag {
	out := map[string][]completionFlag{}
	for _, c := range commands() {
		if c.name == "completion" {
			continue
		}
		var help bytes.Buffer
		c.run(ctx, &Env{Stdout: io.Discard, Stderr: &help, Getenv: func(string) (string, bool) { return "", false }}, []string{"-h"})
		out[c.name] = parseFlagHelp(help.String())
	}
	return out
}

// parseFlagHelp reads flag.PrintDefaults output: "  -name type" followed by
// an indented description line.
func parseFlagHelp(help string) []completionFlag {
	var out []completionFlag
	lines := strings.Split(help, "\n")
	for i, l := range lines {
		if !strings.HasPrefix(l, "  -") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(l, "  -"), " ")
		f := completionFlag{name: name}
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "    \t") {
			f.help = strings.TrimSpace(lines[i+1])
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func commandNames() []string {
	cmds := commands()
	names := make([]string, 0, len(cmds))
	for _, c := range cmds {
		names = append(names, c.name)
	}
	return names
}

func writeBashCompletion(w io.Writer, flags map[string][]completionFlag) {
	fmt.Fprintln(w, "# prodshadow completion for bash and zsh (via bashcompinit)")
	fmt.Fprintln(w, "_prodshadow() {")
	fmt.Fprintln(w, `  local cur="${COMP_WORDS[COMP_CWORD]}" opts=""`)
	fmt.Fprintln(w, `  if [ "$COMP_CWORD" -eq 1 ]; then`)
	fmt.Fprintf(w, "    COMPREPLY=($(compgen -W %q -- \"$cur\"))\n", strings.Join(commandNames(), " "))
	fmt.Fprintln(w, "    return")
	fmt.Fprintln(w, "  fi")
	fmt.Fprintln(w, `  case "${COMP_WORDS[1]}" in`)
	fmt.Fprintf(w, "    schema) COMPREPLY=($(compgen -W %q -- \"$cur\")); return ;;\n", strings.Join(schemas.Names(), " "))
	fmt.Fprintln(w, "    completion) COMPREPLY=($(compgen -W \"bash zsh fish\" -- \"$cur\")); return ;;")
	for _, name := range commandNames() {
		if len(flags[name]) == 0 {
			continue
		}
		opts := make([]string, 0, len(flags[name]))
		for _, f := range flags[name] {
			opts = append(opts, "--"+f.name)
		}
		fmt.Fprintf(w, "    %s) opts=%q ;;\n", name, strings.Join(opts, " "))
	}
	fmt.Fprintln(w, "  esac")
	fmt.Fprintln(w, `  if [[ "$cur" == -* ]]; then`)
	fmt.Fprintln(w, `    COMPREPLY=($(compgen -W "$opts" -- "$cur"))`)
	fmt.Fprintln(w, "  fi")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "complete -o bashdefault -o default -F _prodshadow prodshadow")
}

func writeFishCompletion(w io.Writer, flags map[string][]completionFlag) {
	fmt.Fprintln(w, "# prodshadow completion for fish")
	fmt.Fprintln(w, "complete -c prodshadow -f -n __fish_use_subcommand -a help -d 'Show usage'")
	for _, c := range commands() {
		fmt.Fprintf(w, "complete -c prodshadow -f -n __fish_use_subcommand -a %s -d %s\n", c.name, fishQuote(c.summary))
	}
	for _, s := range schemas.Names() {
		fmt.Fprintf(w, "complete -c prodshadow -f -n '__fish_seen_subcommand_from schema' -a %s\n", s)
	}
	fmt.Fprintln(w, "complete -c prodshadow -f -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'")
	for _, name := range commandNames() {
		for _, f := range flags[name] {
			fmt.Fprintf(w, "complete -c prodshadow -n '__fish_seen_subcommand_from %s' -l %s -d %s\n", name, f.name, fishQuote(f.help))
		}
	}
}

func fishQuote(s string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`) + "'"
}
