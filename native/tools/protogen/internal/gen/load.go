package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// loadMode is the minimum go/packages mode that gives fully type-checked
// syntax: resolved types for every expression plus the AST to walk. Anything
// less would force protogen back onto syntax guessing, which is exactly what a
// regex extractor does wrong.
const loadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedCompiledGoFiles |
	packages.NeedImports |
	packages.NeedDeps |
	packages.NeedTypes |
	packages.NeedSyntax |
	packages.NeedTypesInfo

// Program is a loaded, type-checked Go program plus the indexes protogen needs
// to walk from a route registration to a handler body to a DTO.
type Program struct {
	// Fset is the shared position table.
	Fset *token.FileSet
	// Pkgs is every loaded package, keyed by import path.
	Pkgs map[string]*packages.Package
	// Roots is the packages matched by the load patterns, in load order.
	Roots []*packages.Package

	// funcDecls maps a func object to its declaration, so a resolved
	// method value (h.Status) can be walked as source.
	funcDecls map[types.Object]*funcSite
	// typeDocs maps a named type object to its doc comment.
	typeDocs map[types.Object]string
	// constsByType maps a named type to its package-level constants, in
	// declaration order, for string-enum detection.
	constsByType map[types.Object][]constSite
	// fieldDocs maps a struct field's declaration position to its doc
	// comment. go/types drops comments, so they are recovered from syntax.
	fieldDocs map[token.Pos]string
}

// funcSite is a function declaration together with the package it belongs to,
// because reading its body needs that package's TypesInfo.
type funcSite struct {
	// Decl is the declaration.
	Decl *ast.FuncDecl
	// Pkg is the owning package.
	Pkg *packages.Package
}

// constSite is one package-level constant of a named type.
type constSite struct {
	// Name is the Go identifier.
	Name string
	// Value is the constant's string value.
	Value string
	// Doc is the constant's doc comment.
	Doc string
}

// Load type-checks every package under the given patterns rooted at dir.
// It returns an error only when loading itself fails; per-package type errors
// are reported through the returned Program's Roots so a partially broken tree
// still yields whatever it can.
func Load(
	dir string,
	patterns ...string,
) (*Program, error) {
	cfg := &packages.Config{
		Mode: loadMode,
		Dir:  dir,
		// Tests are irrelevant to the wire surface and pull in fixtures that
		// would pollute the discovered type set.
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load %v in %s: %w", patterns, dir, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("load %v in %s: no packages matched", patterns, dir)
	}

	p := &Program{
		Fset:         pkgs[0].Fset,
		Pkgs:         map[string]*packages.Package{},
		Roots:        pkgs,
		funcDecls:    map[types.Object]*funcSite{},
		typeDocs:     map[types.Object]string{},
		constsByType: map[types.Object][]constSite{},
		fieldDocs:    map[token.Pos]string{},
	}
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		p.Pkgs[pkg.PkgPath] = pkg
		p.index(pkg)
	})
	return p, nil
}

// index records the declarations of one package into the program-wide maps.
func (p *Program) index(
	pkg *packages.Package,
) {
	if pkg.TypesInfo == nil {
		return
	}
	for _, file := range pkg.Syntax {
		for _, d := range file.Decls {
			switch decl := d.(type) {
			case *ast.FuncDecl:
				p.indexFunc(pkg, decl)
			case *ast.GenDecl:
				p.indexGenDecl(pkg, decl)
			}
		}
	}
}

// indexFunc records a function or method declaration under its type object.
func (p *Program) indexFunc(
	pkg *packages.Package,
	decl *ast.FuncDecl,
) {
	if decl.Name == nil {
		return
	}
	obj, ok := pkg.TypesInfo.Defs[decl.Name]
	if !ok || obj == nil {
		return
	}
	p.funcDecls[obj] = &funcSite{Decl: decl, Pkg: pkg}
}

// indexGenDecl records type specs (with docs) and typed string constants.
func (p *Program) indexGenDecl(
	pkg *packages.Package,
	decl *ast.GenDecl,
) {
	switch decl.Tok {
	case token.TYPE:
		for _, s := range decl.Specs {
			ts, ok := s.(*ast.TypeSpec)
			if !ok {
				continue
			}
			obj := pkg.TypesInfo.Defs[ts.Name]
			if obj == nil {
				continue
			}
			doc := ts.Doc
			if doc == nil {
				doc = decl.Doc
			}
			p.typeDocs[obj] = commentText(doc)
			p.indexFieldDocs(ts.Type)
		}
	case token.CONST:
		p.indexConsts(pkg, decl)
	}
}

// indexFieldDocs records the doc comment of every field of a struct type
// expression, keyed by the field name's position — the same position go/types
// reports for the corresponding *types.Var.
func (p *Program) indexFieldDocs(
	expr ast.Expr,
) {
	st, ok := expr.(*ast.StructType)
	if !ok || st.Fields == nil {
		return
	}
	for _, field := range st.Fields.List {
		doc := commentText(field.Doc)
		if doc == "" && field.Comment != nil {
			doc = commentText(field.Comment)
		}
		if doc == "" {
			continue
		}
		if len(field.Names) == 0 {
			continue
		}
		for _, name := range field.Names {
			p.fieldDocs[name.Pos()] = doc
		}
	}
}

// indexConsts records package-level string constants grouped by their named
// type. A `const ( X T = "x" ... )` block is what makes T an enum.
func (p *Program) indexConsts(
	pkg *packages.Package,
	decl *ast.GenDecl,
) {
	for _, s := range decl.Specs {
		vs, ok := s.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range vs.Names {
			obj, _ := pkg.TypesInfo.Defs[name].(*types.Const)
			if obj == nil || obj.Val() == nil {
				continue
			}
			named, ok := obj.Type().(*types.Named)
			if !ok {
				continue
			}
			basic, ok := named.Underlying().(*types.Basic)
			if !ok || basic.Kind() != types.String {
				continue
			}
			doc := vs.Doc
			if doc == nil {
				doc = decl.Doc
			}
			p.constsByType[named.Obj()] = append(
				p.constsByType[named.Obj()],
				constSite{
					Name:  obj.Name(),
					Value: constStringValue(obj),
					Doc:   commentText(doc),
				},
			)
		}
	}
}

// FuncBody returns the declaration and owning package of a resolved function
// object, or nil when the function has no Go source in the loaded set (a
// builtin, or a dependency loaded without syntax).
func (p *Program) FuncBody(
	obj types.Object,
) *funcSite {
	return p.funcDecls[obj]
}

// TypeDoc returns the doc comment of a named type.
func (p *Program) TypeDoc(
	obj types.Object,
) string {
	return p.typeDocs[obj]
}

// EnumConsts returns the string constants declared for a named type, sorted by
// value so output ordering never depends on declaration order across files.
func (p *Program) EnumConsts(
	obj types.Object,
) []constSite {
	got := p.constsByType[obj]
	if len(got) == 0 {
		return nil
	}
	out := make([]constSite, len(got))
	copy(out, got)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value < out[j].Value
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Pos renders a source position relative to root, so the manifest is portable
// across checkouts.
func (p *Program) Pos(
	pos token.Pos,
	root string,
) string {
	if !pos.IsValid() {
		return ""
	}
	pp := p.Fset.Position(pos)
	file := pp.Filename
	if root != "" {
		if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
			file = rel
		}
	}
	return fmt.Sprintf("%s:%d", filepath.ToSlash(file), pp.Line)
}

// commentText renders a doc comment group without its markers.
func commentText(
	g *ast.CommentGroup,
) string {
	if g == nil {
		return ""
	}
	return strings.TrimRight(g.Text(), "\n")
}

// constStringValue extracts the Go string value of a string constant.
func constStringValue(
	c *types.Const,
) string {
	s := c.Val().String()
	// constant.Value.String quotes strings; strip the quotes without
	// re-parsing, because the values here are plain identifiers-with-dashes.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		unquoted, err := unquoteGo(s)
		if err == nil {
			return unquoted
		}
		return s[1 : len(s)-1]
	}
	return s
}
