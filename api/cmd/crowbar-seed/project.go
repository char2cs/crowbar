package main

import "context"

const (
	seedProjectName = "Crowbar Seed"
	projectsPath    = "/v0/projects"
)

type projectDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func pickProject(
	list []projectDTO,
) (projectDTO, bool) {
	for _, p := range list {
		if p.Name == seedProjectName {
			return p, true
		}
	}
	return projectDTO{}, false
}

func ensureProject(
	ctx context.Context,
	d *daemon,
	path string,
) (projectDTO, bool, error) {
	existing, err := getData[[]projectDTO](ctx, d, "list projects", projectsPath)
	if err != nil {
		return projectDTO{}, false, err
	}
	if found, ok := pickProject(existing); ok {
		return found, false, nil
	}
	body := map[string]any{"name": seedProjectName, "path": path}
	if err := d.postAccepted(ctx, "import project", projectsPath, body); err != nil {
		return projectDTO{}, false, err
	}
	created, err := waitFor(ctx, d, "the seed project", projectsPath, pickProject)
	return created, true, err
}
