package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestFileNodeDTOFromRecursesChildren(
	t *testing.T,
) {
	status := domain.FileNodeGitStatusModified
	node := domain.FileNode{
		Name: "src",
		Path: "src",
		Type: domain.FileNodeTypeDirectory,
		Children: []domain.FileNode{
			{
				Name:      "main.go",
				Path:      "src/main.go",
				Type:      domain.FileNodeTypeFile,
				GitStatus: &status,
			},
		},
	}

	got := dto.FileNodeDTOFrom(node)

	assert.Equal(t, "src", got.Name)
	assert.Equal(t, domain.FileNodeTypeDirectory, got.Type)
	assert.Nil(t, got.GitStatus)
	assert.Len(t, got.Children, 1)
	assert.Equal(t, "src/main.go", got.Children[0].Path)
	assert.Equal(t, &status, got.Children[0].GitStatus)
}

func TestFileNodeDTOListEmptyIsNil(
	t *testing.T,
) {
	assert.Nil(t, dto.FileNodeDTOList(nil))
}

func TestFileNodeDTOListConverts(
	t *testing.T,
) {
	nodes := []domain.FileNode{
		{Name: "a", Path: "a", Type: domain.FileNodeTypeFile},
		{Name: "b", Path: "b", Type: domain.FileNodeTypeFile},
	}

	got := dto.FileNodeDTOList(nodes)

	assert.Len(t, got, 2)
	assert.Equal(t, "a", got[0].Name)
	assert.Equal(t, "b", got[1].Name)
}

func TestFileContentDTOFrom(
	t *testing.T,
) {
	got := dto.FileContentDTOFrom(domain.FileContent{Content: "x", Encoding: "base64"})

	assert.Equal(t, "x", got.Content)
	assert.Equal(t, "base64", got.Encoding)
}
