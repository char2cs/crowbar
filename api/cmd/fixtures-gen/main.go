package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
)

type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Type     string     `json:"type"`
	Size     int64      `json:"size,omitempty"`
	Children []FileNode `json:"children,omitempty"`
}

type Commit struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"shortHash"`
	Message   string `json:"message"`
	Author    string `json:"author"`
	Date      string `json:"date"`
}

var commitMsgs = []string{
	"feat: add payment processing module",
	"fix: resolve null pointer in auth middleware",
	"refactor: extract database connection pool",
	"chore: update dependencies to latest versions",
	"docs: add API endpoint documentation",
	"test: add integration tests for user service",
	"perf: optimize query execution with indexes",
	"feat: implement real-time notification system",
	"fix: handle edge case in date parsing",
	"Merge branch 'feature/payments' into develop",
	"feat: add CSV export functionality",
	"fix: correct timezone handling in scheduler",
	"chore: add CI/CD pipeline configuration",
	"style: apply linting rules across codebase",
	"feat: add dashboard analytics widgets",
}

var authors = []string{"Mateo Urrutia", "Claude Agent", "Dependabot[bot]"}

var exts = []string{".ts", ".tsx", ".go", ".json", ".md", ".yaml", ".css", ".test.ts"}

var dirNames = []string{
	"components", "utils", "services", "models", "hooks",
	"api", "lib", "types", "store", "features", "tests", "docs",
}

func generateTree(path string, depth, maxDepth int, count, target *int) FileNode {
	node := FileNode{Name: filepath.Base(path), Path: path, Type: "directory"}
	if depth >= maxDepth || *count >= *target {
		return node
	}
	n := rand.Intn(6) + 2
	for i := 0; i < n && *count < *target; i++ {
		(*count)++
		if depth < maxDepth-1 && rand.Float32() < 0.35 {
			dirName := fmt.Sprintf("%s-%d", dirNames[rand.Intn(len(dirNames))], i)
			child := generateTree(path+"/"+dirName, depth+1, maxDepth, count, target)
			node.Children = append(node.Children, child)
		} else {
			ext := exts[rand.Intn(len(exts))]
			fname := fmt.Sprintf("file-%d%s", i, ext)
			node.Children = append(node.Children, FileNode{
				Name: fname,
				Path: path + "/" + fname,
				Type: "file",
				Size: int64(rand.Intn(200000) + 100),
			})
		}
	}
	return node
}

func generateLog(n int) []Commit {
	commits := make([]Commit, n)
	for i := range commits {
		h := fmt.Sprintf("%040x", rand.Int63()^int64(i))
		commits[i] = Commit{
			Hash:      h,
			ShortHash: h[:7],
			Message:   commitMsgs[rand.Intn(len(commitMsgs))],
			Author:    authors[rand.Intn(len(authors))],
			Date:      fmt.Sprintf("%d days ago", i/10+1),
		}
	}
	return commits
}

func writeJSON(path string, v any) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
}

func main() {
	outDir := "api/internal/fixtures"

	count, target := 0, 5000
	tree := generateTree("crowbar", 0, 7, &count, &target)
	writeJSON(filepath.Join(outDir, "file-tree.json"), tree)
	fmt.Printf("file-tree.json: %d nodes\n", count)

	log := generateLog(2000)
	writeJSON(filepath.Join(outDir, "git-log.json"), log)
	fmt.Printf("git-log.json: %d commits\n", len(log))
}
