package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/BhuwanJoshi-01/Open-Source-1/schemas"
)

func runSchema(_ context.Context, env *Env, args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintf(env.Stderr, "Usage: prodshadow schema <%s>\n", strings.Join(schemas.Names(), "|"))
		if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			return ExitOK
		}
		return ExitInvalid
	}
	raw, err := schemas.Get(args[0])
	if err != nil {
		fmt.Fprintln(env.Stderr, err)
		return ExitInvalid
	}
	env.Stdout.Write(raw)
	return ExitOK
}
