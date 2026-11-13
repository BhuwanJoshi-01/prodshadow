package experiment

import (
	"context"
	"fmt"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/fixture"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/postgres"
)

// seedTemplate creates and fills the template database. Extensions are
// installed by the administrative role; schema, seed and generated rows are
// created by the unprivileged application role, which owns the database.
func seedTemplate(ctx context.Context, plan Plan, srv *isolation.Server, res *Result) error {
	cfg := plan.Config
	app, _ := srv.App()

	admin, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{ApplicationName: "prodshadow/admin"})
	if err != nil {
		return err
	}
	defer admin.Close(ctx)
	if err := postgres.CreateDatabase(ctx, admin, TemplateDatabase, app.User, ""); err != nil {
		return err
	}

	tplAdmin, err := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{Database: TemplateDatabase, ApplicationName: "prodshadow/admin"})
	if err != nil {
		return err
	}
	for _, ext := range cfg.Postgres.Extensions {
		if err := postgres.CreateExtension(ctx, tplAdmin, ext); err != nil {
			tplAdmin.Close(ctx)
			return err
		}
	}

	owner, err := srv.Connect(ctx, app, isolation.ConnOptions{Database: TemplateDatabase, ApplicationName: "prodshadow/fixture"})
	if err != nil {
		tplAdmin.Close(ctx)
		return err
	}
	if plan.Schema != nil {
		if _, err := fixture.ApplySQL(ctx, owner, *plan.Schema); err != nil {
			return closeBoth(ctx, owner, tplAdmin, err)
		}
	}
	if plan.Seed != nil {
		if _, err := fixture.ApplySQL(ctx, owner, *plan.Seed); err != nil {
			return closeBoth(ctx, owner, tplAdmin, err)
		}
	}
	if plan.Generator != nil {
		rows, err := fixture.Generate(ctx, owner, plan.Generator, cfg.Fixture.Generator.Seed)
		res.FixtureRows = rows
		if err != nil {
			return closeBoth(ctx, owner, tplAdmin, err)
		}
	}
	if _, err := owner.Exec(ctx, "ANALYZE"); err != nil {
		return closeBoth(ctx, owner, tplAdmin, fmt.Errorf("analyze fixture: %w", err))
	}
	owner.Close(ctx)

	if res.Server, err = postgres.Introspect(ctx, tplAdmin); err != nil {
		tplAdmin.Close(ctx)
		return err
	}
	if res.Tables, err = postgres.TableSizes(ctx, tplAdmin); err != nil {
		tplAdmin.Close(ctx)
		return err
	}
	// A template cannot be copied while sessions are connected to it.
	return tplAdmin.Close(ctx)
}

func closeBoth(ctx context.Context, a, b interface{ Close(context.Context) error }, err error) error {
	a.Close(ctx)
	b.Close(ctx)
	return err
}
