package project

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/avatar"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type RepoUpdate struct {
	Name      *string
	ProjectID *string
	Order     *int
	FolderID  *string
}

func (u *projectUsecase) UpdateRepo(
	ctx context.Context,
	repoID string,
	in RepoUpdate,
) error {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil {
		return fmt.Errorf("project: update repo: %w", err)
	}
	if repo == nil {
		return fmt.Errorf("project: update repo: id %s: %w", repoID, problem.ErrNotFound)
	}
	if err := refuseProjectMove(*repo, in.ProjectID); err != nil {
		return err
	}
	subject, err := u.getRepoNode(ctx, repoID)
	if err != nil {
		return fmt.Errorf("project: update repo: node: %w", err)
	}
	if in.Name != nil {
		repo.Name = *in.Name
		repo.AvatarLabel = avatar.Label(*in.Name)
		repo.AvatarColor = avatar.Color(*in.Name)
	}
	originFolder := subject.ParentID
	targetFolder, err := u.resolveTargetFolder(ctx, in, originFolder)
	if err != nil {
		return err
	}
	if err := u.repos.Save(ctx, *repo); err != nil {
		return fmt.Errorf("project: update repo: save: %w", err)
	}
	if _, err := u.applyRepoPlacement(ctx, *repo, targetFolder, subject, in.Order); err != nil {
		return err
	}
	if originFolder != targetFolder {
		if _, err := u.densifyHomeLevel(ctx, repo.ProjectID, originFolder, repoID); err != nil {
			return err
		}
	}
	return nil
}

func (u *projectUsecase) getRepoNode(
	ctx context.Context,
	repoID string,
) (tree.Node, error) {
	n, err := u.nodes.GetNode(ctx, repoID)
	if err != nil {
		return tree.Node{}, err
	}
	return n, nil
}

func (u *projectUsecase) resolveTargetFolder(
	ctx context.Context,
	in RepoUpdate,
	originFolder string,
) (string, error) {
	if in.FolderID == nil {
		return originFolder, nil
	}
	if err := u.validateRepoFolder(ctx, *in.FolderID); err != nil {
		return "", err
	}
	return *in.FolderID, nil
}

func (u *projectUsecase) applyRepoPlacement(
	ctx context.Context,
	repo domain.Repository,
	targetFolder string,
	subject tree.Node,
	order *int,
) ([]tree.Node, error) {
	if order != nil {
		return u.placeRepoAmongHomeSiblings(ctx, repo.ProjectID, targetFolder, subject, *order)
	}
	return u.densifyRepos(ctx, repo.ProjectID, targetFolder, &subject)
}

func (u *projectUsecase) validateRepoFolder(
	ctx context.Context,
	folderID string,
) error {
	if folderID == "" {
		return nil
	}
	folder, err := u.folders.FindByKey(ctx, folderID)
	if err != nil {
		return fmt.Errorf("project: update repo: folder %s: %w", folderID, err)
	}
	if folder == nil || folder.RepoID != "" {
		return fmt.Errorf(
			"project: update repo: %s is not a project-home folder: %w", folderID, problem.ErrInvalidArgument,
		)
	}
	return nil
}

func refuseProjectMove(
	repo domain.Repository,
	projectID *string,
) error {
	if projectID == nil || *projectID == repo.ProjectID {
		return nil
	}
	return fmt.Errorf(
		"project: update repo: a repository cannot change projects; remove it and import it into %s: %w",
		*projectID, problem.ErrConflict,
	)
}

func (u *projectUsecase) repoIDSet(
	ctx context.Context,
	projectID string,
) (map[string]bool, error) {
	rows, err := u.repos.FindWhere(ctx, domain.Repository{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("project: reorder repos: list: %w", err)
	}
	ids := make(map[string]bool, len(rows))
	for _, r := range rows {
		ids[r.ID] = true
	}
	return ids, nil
}

func (u *projectUsecase) densifyRepos(
	ctx context.Context,
	projectID string,
	folderID string,
	subject *tree.Node,
) ([]tree.Node, error) {
	memberIDs, err := u.repoIDSet(ctx, projectID)
	if err != nil {
		return nil, err
	}
	nodes, err := u.nodes.ListByParent(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("project: reorder repos: list nodes: %w", err)
	}
	rows := densifyRows(memberIDs, nodes, subject)
	subjectID := ""
	if subject != nil {
		subjectID = subject.ID
	}
	slots := nodeIndex(rows)
	written := make(map[string]bool, len(rows))
	var decided []tree.Node
	for _, moved := range place(slots, subjectID, nil) {
		row := rows[moved.at]
		written[row.ID] = true
		decided = append(decided, tree.Node{ID: row.ID, Kind: row.Kind, ParentID: folderID, Order: moved.order})
		reparenting := subject != nil && row.ID == subject.ID && subject.ParentID != folderID
		if err := u.writeNode(ctx, row.ID, folderID, moved.order, reparenting); err != nil {
			return nil, err
		}
	}
	if subject == nil {
		return decided, nil
	}
	n, err := u.ensureSubjectWritten(ctx, slots, subject.ID, subject.ParentID != folderID, folderID, nil, written)
	if err != nil {
		return nil, err
	}
	if n != nil {
		decided = append(decided, *n)
	}
	return decided, nil
}

func densifyRows(
	memberIDs map[string]bool,
	nodes []tree.Node,
	subject *tree.Node,
) []tree.Node {
	rows := make([]tree.Node, 0, len(nodes)+1)
	for _, n := range nodes {
		if !memberIDs[n.ID] {
			continue
		}
		if subject != nil && n.ID == subject.ID {
			continue
		}
		rows = append(rows, n)
	}
	if subject != nil {
		rows = append(rows, *subject)
	}
	return rows
}

func (u *projectUsecase) writeNode(
	ctx context.Context,
	id string,
	folderID string,
	order int,
	reparenting bool,
) error {
	if reparenting {
		if err := u.nodes.SetPlacement(ctx, id, folderID, order); err != nil {
			return fmt.Errorf("project: reorder repos: place %s: %w", id, err)
		}
		return nil
	}
	if err := u.nodes.SetOrder(ctx, id, order); err != nil {
		return fmt.Errorf("project: reorder repos: save %s: %w", id, err)
	}
	return nil
}

func (u *projectUsecase) mintNode(
	ctx context.Context,
	id string,
	kind tree.Kind,
	folderID string,
	order int,
) error {
	if _, err := u.nodes.Create(ctx, id, kind, folderID, order); err != nil {
		return fmt.Errorf("project: reorder repos: mint %s: %w", id, err)
	}
	return nil
}

func (u *projectUsecase) ensureSubjectWritten(
	ctx context.Context,
	slots []slot,
	subjectID string,
	reparented bool,
	folderID string,
	target *int,
	written map[string]bool,
) (*tree.Node, error) {
	if written[subjectID] {
		return nil, nil
	}
	i := finalIndexOf(slots, subjectID, target)
	if !reparented || i < 0 {
		return nil, nil
	}
	if err := u.nodes.SetPlacement(ctx, subjectID, folderID, i); err != nil {
		return nil, fmt.Errorf("project: reorder repos: place %s: %w", subjectID, err)
	}
	return &tree.Node{ID: subjectID, Kind: tree.KindRepo, ParentID: folderID, Order: i}, nil
}
