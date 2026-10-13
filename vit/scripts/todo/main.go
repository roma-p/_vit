package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type hit struct {
	file string
	line int
	fn   string
	msg  string
}

var useColor bool

const (
	colFile = "36"   // cyan
	colFunc = "33"   // yellow
	colLine = "90"   // grey
	colTag  = "1;31" // bold red
)

func paint(code, s string) string {
	if !useColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func paintTag(msg string) string {
	for _, kw := range []string{"TODO", "FIXME"} {
		if strings.HasPrefix(msg, kw) {
			return paint(colTag, kw) + msg[len(kw):]
		}
	}
	return msg
}

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	useColor = colorEnabled()

	fset := token.NewFileSet()
	var hits []hit

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			return nil
		}
		hits = append(hits, scanFile(fset, path, f)...)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "TODO:", err)
		os.Exit(1)
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].file != hits[j].file {
			return hits[i].file < hits[j].file
		}
		return hits[i].line < hits[j].line
	})

	nFiles := 0
	for i := 0; i < len(hits); {
		file := hits[i].file
		j := i
		for j < len(hits) && hits[j].file == file {
			j++
		}
		printFile(file, hits[i:j])
		nFiles++
		i = j
	}
	fmt.Printf("%d TODO marker(s) across %d file(s)\n\n", len(hits), nFiles)
}

func printFile(file string, fileHits []hit) {
	lineTag := func(n int) string { return paint(colLine, fmt.Sprintf(":%d", n)) }

	if len(fileHits) == 1 && fileHits[0].fn == "<file>" {
		h := fileHits[0]
		fmt.Printf("%s%s -> %s\n", paint(colFile, file), lineTag(h.line), paintTag(h.msg))
		return
	}

	var order []string
	byFn := map[string][]hit{}
	for _, h := range fileHits {
		if _, seen := byFn[h.fn]; !seen {
			order = append(order, h.fn)
		}
		byFn[h.fn] = append(byFn[h.fn], h)
	}

	fmt.Println(paint(colFile, file))
	for _, fn := range order {
		ts := byFn[fn]
		if len(ts) == 1 {
			fmt.Printf("  %s%s -> %s\n", paint(colFunc, fn), lineTag(ts[0].line), paintTag(ts[0].msg))
			continue
		}
		fmt.Printf("  %s:\n", paint(colFunc, fn))
		for _, t := range ts {
			fmt.Printf("    %s  %s\n", lineTag(t.line), paintTag(t.msg))
		}
	}
}

func scanFile(fset *token.FileSet, path string, f *ast.File) []hit {
	type fnRange struct {
		name       string
		start, end token.Pos
	}
	var funcs []fnRange
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			funcs = append(funcs, fnRange{funcName(fd), fd.Pos(), fd.End()})
		}
	}
	enclosing := func(p token.Pos) string {
		for _, fr := range funcs {
			if p >= fr.start && p < fr.end {
				return fr.name
			}
		}
		return "<file>"
	}

	var out []hit

	for _, cg := range f.Comments {
		for _, c := range cg.List {
			body := strings.TrimLeft(c.Text, "/* \t")
			if !strings.HasPrefix(body, "TODO") && !strings.HasPrefix(body, "FIXME") {
				continue
			}
			pos := fset.Position(c.Pos())
			out = append(out, hit{path, pos.Line, enclosing(c.Pos()), strings.TrimSpace(body)})
		}
	}

	return out
}

func funcName(fd *ast.FuncDecl) string {
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		recv := ""
		switch e := fd.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			if id, ok := e.X.(*ast.Ident); ok {
				recv = id.Name
			}
		case *ast.Ident:
			recv = e.Name
		}
		if recv != "" {
			return recv + "." + fd.Name.Name
		}
	}
	return fd.Name.Name
}
