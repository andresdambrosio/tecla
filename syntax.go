package main

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
)

type Lang struct {
	Name   string
	Line   []string // comentarios de línea
	BS, BE string   // comentario de bloque
	Quotes string
	KW     map[string]bool
	NoCase bool
}

func kw(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

var langs = map[string]*Lang{
	"go": {Name: "Go", Line: []string{"//"}, BS: "/*", BE: "*/", Quotes: "\"'`", KW: kw(`break case chan const continue default defer else
		fallthrough for func go goto if import interface map package range return select struct switch type var
		nil true false iota string int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64 byte rune bool error
		float32 float64 any make new len cap append panic recover copy delete close`)},
	"python": {Name: "Python", Line: []string{"#"}, BS: `"""`, BE: `"""`, Quotes: "\"'", KW: kw(`and as assert async await break class
		continue def del elif else except False finally for from global if import in is lambda None nonlocal not or
		pass raise return True try while with yield self print len range match case`)},
	"js": {Name: "JavaScript/TypeScript", Line: []string{"//"}, BS: "/*", BE: "*/", Quotes: "\"'`", KW: kw(`break case catch
		class const continue debugger default delete do else export extends false finally for function if import in
		instanceof let new null return super switch this throw true try typeof undefined var void while yield async
		await of interface type enum implements private public protected readonly static as from`)},
	"c": {Name: "C/C++", Line: []string{"//"}, BS: "/*", BE: "*/", Quotes: "\"'", KW: kw(`auto break case char const continue
		default do double else enum extern float for goto if int long register return short signed sizeof static
		struct switch typedef union unsigned void volatile while include define ifdef ifndef endif pragma class
		public private protected namespace template typename new delete this true false nullptr bool virtual override
		using std NULL`)},
	"rust": {Name: "Rust", Line: []string{"//"}, BS: "/*", BE: "*/", Quotes: "\"", KW: kw(`as async await break const continue
		crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static
		struct super trait true type unsafe use where while Some None Ok Err String Vec Option Result`)},
	"java": {Name: "Java/Kotlin", Line: []string{"//"}, BS: "/*", BE: "*/", Quotes: "\"'", KW: kw(`abstract boolean break byte
		case catch char class const continue default do double else enum extends final finally float for if
		implements import instanceof int interface long new package private protected public return short static
		super switch this throw throws try void while true false null fun val var when object`)},
	"swift": {Name: "Swift", Line: []string{"//"}, BS: "/*", BE: "*/", Quotes: "\"", KW: kw(`class deinit enum extension func
		import init let protocol struct subscript typealias var break case continue default do else fallthrough for
		guard if in repeat return switch where while as false is nil self Self super throw throws true try catch
		some any async await private public internal static`)},
	"ruby": {Name: "Ruby", Line: []string{"#"}, Quotes: "\"'", KW: kw(`alias and begin break case class def defined do else
		elsif end ensure false for if in module next nil not or redo rescue retry return self super then true undef
		unless until when while yield require puts attr_accessor`)},
	"lua": {Name: "Lua", Line: []string{"--"}, Quotes: "\"'", KW: kw(`and break do else elseif end false for function goto if
		in local nil not or repeat return then true until while`)},
	"sh": {Name: "Shell", Line: []string{"#"}, Quotes: "\"'", KW: kw(`if then else elif fi for while until do done case esac
		function in return export local echo exit source alias set unset read shift`)},
	"sql": {Name: "SQL", Line: []string{"--"}, BS: "/*", BE: "*/", Quotes: "'\"", NoCase: true, KW: kw(`select from where
		insert into values update set delete create table drop alter index join left right inner outer on and or not
		null is as order by group having limit offset distinct union all primary key foreign references default
		begin commit rollback case when then else end in exists like between count sum avg min max`)},
	"html": {Name: "HTML/XML", BS: "<!--", BE: "-->", Quotes: "\"'"},
	"css":  {Name: "CSS", BS: "/*", BE: "*/", Quotes: "\"'"},
	"conf": {Name: "Config", Line: []string{"#"}, Quotes: "\"'", KW: kw(`true false null yes no`)},
}

var extLang = map[string]string{
	".go": "go", ".py": "python", ".js": "js", ".mjs": "js", ".cjs": "js", ".ts": "js", ".tsx": "js", ".jsx": "js",
	".json": "js", ".c": "c", ".h": "c", ".cpp": "c", ".hpp": "c", ".cc": "c", ".m": "c", ".rs": "rust",
	".java": "java", ".kt": "java", ".swift": "swift", ".rb": "ruby", ".lua": "lua", ".sh": "sh", ".bash": "sh",
	".zsh": "sh", ".sql": "sql", ".html": "html", ".htm": "html", ".xml": "html", ".svg": "html", ".css": "css",
	".scss": "css", ".toml": "conf", ".yaml": "conf", ".yml": "conf", ".ini": "conf", ".env": "conf",
}

func langFor(path string) *Lang {
	if path == "" {
		return nil
	}
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "makefile", "dockerfile", ".bashrc", ".zshrc", ".profile":
		return langs["sh"]
	}
	if l, ok := extLang[filepath.Ext(base)]; ok {
		return langs[l]
	}
	return nil
}

func (l *Lang) lineComment() string {
	if l == nil {
		return ""
	}
	if len(l.Line) > 0 {
		return l.Line[0]
	}
	return ""
}

func hasPrefixAt(line []rune, i int, p string) bool {
	for _, r := range p {
		if i >= len(line) || line[i] != r {
			return false
		}
		i++
	}
	return true
}

func indexFrom(line []rune, p string, from int) int {
	for i := from; i < len(line); i++ {
		if hasPrefixAt(line, i, p) {
			return i
		}
	}
	return -1
}

var noTheme = &Theme{}

// highlight colorea una línea. Si out es nil, sólo calcula el estado de comentario de bloque.
func highlight(line []rune, l *Lang, in bool, T *Theme, out []tcell.Style) bool {
	if T == nil {
		T = noTheme
	}
	n := len(line)
	set := func(a, b int, s tcell.Style) {
		if out == nil {
			return
		}
		for k := a; k < b && k < n; k++ {
			out[k] = s
		}
	}
	set(0, n, T.Normal)
	if l == nil {
		return false
	}
	bs, be := len([]rune(l.BS)), len([]rune(l.BE))
	i := 0
	for i < n {
		if in {
			j := indexFrom(line, l.BE, i)
			if j < 0 {
				set(i, n, T.Comment)
				return true
			}
			set(i, j+be, T.Comment)
			i = j + be
			in = false
			continue
		}
		c := line[i]
		if l.BS != "" && hasPrefixAt(line, i, l.BS) {
			in = true
			set(i, i+bs, T.Comment)
			i += bs
			continue
		}
		for _, p := range l.Line {
			if hasPrefixAt(line, i, p) {
				set(i, n, T.Comment)
				return false
			}
		}
		if strings.ContainsRune(l.Quotes, c) {
			j := i + 1
			for j < n && line[j] != c {
				if line[j] == '\\' {
					j++
				}
				j++
			}
			if j < n {
				j++
			} else {
				j = n
			}
			set(i, j, T.String)
			i = j
			continue
		}
		if unicode.IsDigit(c) && (i == 0 || !isIdent(line[i-1])) {
			j := i
			for j < n && (isIdent(line[j]) || line[j] == '.') {
				j++
			}
			set(i, j, T.Number)
			i = j
			continue
		}
		if c == '_' || unicode.IsLetter(c) {
			j := i
			for j < n && isIdent(line[j]) {
				j++
			}
			w := string(line[i:j])
			if l.NoCase {
				w = strings.ToLower(w)
			}
			if l.KW[w] {
				set(i, j, T.Keyword)
			}
			i = j
			continue
		}
		i++
	}
	return in
}
