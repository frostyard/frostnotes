package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// File is one schema/rpc/*.yaml document. errors.yaml holds only Errors;
// every other file declares one domain.
type File struct {
	Domain   string     `yaml:"domain"`
	Protocol int        `yaml:"protocol"`
	Doc      string     `yaml:"doc"`
	Callers  []string   `yaml:"callers"`
	Enums    []Enum     `yaml:"enums"`
	Types    []Type     `yaml:"types"`
	Methods  []Method   `yaml:"methods"`
	Events   []Event    `yaml:"events"`
	Errors   []ErrorDef `yaml:"errors"`

	path string
}

type Enum struct {
	Name   string      `yaml:"name"`
	Doc    string      `yaml:"doc"`
	Values []EnumValue `yaml:"values"`
}

type EnumValue struct {
	Name string `yaml:"name"`
	Doc  string `yaml:"doc"`
}

type Field struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Doc      string `yaml:"doc"`
	Optional bool   `yaml:"optional"`

	ref *TypeRef
}

type Type struct {
	Name   string  `yaml:"name"`
	Doc    string  `yaml:"doc"`
	Fields []Field `yaml:"fields"`
}

type Method struct {
	Name   string   `yaml:"name"`
	Doc    string   `yaml:"doc"`
	Params []Field  `yaml:"params"`
	Result string   `yaml:"result"`
	Errors []string `yaml:"errors"`
	// Callers, when listed, narrow the domain's: each must be one of them.
	Callers []string `yaml:"callers"`
	// Writes marks a method that changes notes, which an agent's read
	// scope refuses (ADR-0026).
	Writes bool `yaml:"writes"`

	result *TypeRef
}

// admits is the callers a method admits: its own list, else its domain's.
func (m Method) admits(f *File) []string {
	if len(m.Callers) > 0 {
		return m.Callers
	}
	return f.Callers
}

type Event struct {
	Name    string  `yaml:"name"`
	Doc     string  `yaml:"doc"`
	Durable bool    `yaml:"durable"`
	Fields  []Field `yaml:"fields"`
}

type ErrorDef struct {
	Name string `yaml:"name"`
	Code int    `yaml:"code"`
	Doc  string `yaml:"doc"`
}

// reservedClientMethods are api.Client's hand-written methods; a domain's
// generated accessor Client.<Domain>() must not shadow them.
var reservedClientMethods = map[string]bool{
	"Call": true, "Close": true, "Done": true, "Err": true, "Notifications": true,
}

// reservedAPINames are package api's hand-written identifiers (api.go,
// client.go, conn.go); generated names must not collide with them.
var reservedAPINames = []string{
	"Caller", "CallerAgent", "CallerDevice", "CallerFrom", "CallerLocal", "CallerSession",
	"Client", "Conn", "ConnFrom", "Dial", "Error", "ErrorCode", "Errorf", "Event", "EventBuffer",
	"EventEnvelope", "EventMethod", "Frame", "HasMethod", "Identity", "IdentityFrom", "MaxMessageSize",
	"NewClient", "NewEnvelope", "NewRouter", "ParseScope", "Router", "Scope", "ErrEventsOverflow",
	"WithCaller", "WithConn", "WithIdentity",
}

// callerKinds are the values a domain's callers may list (ADR-0008,
// ADR-0026): the local socket, a browser lodge signed in, a paired
// device's sync, and an agent behind lodge's /mcp.
var callerKinds = []string{"local", "session", "device", "agent"}

// scopeParam is the param that names a method's vault, which an agent's
// scope checks. Every method an agent may call names one, except the
// connection's own (unscopedDomain).
const (
	scopeParam     = "vaultId"
	unscopedDomain = "rpc"
)

// builtinErrors are the JSON-RPC 2.0 codes; methods may list them by name.
var builtinErrors = []ErrorDef{
	{"parseError", -32700, "The request is not valid JSON."},
	{"invalidRequest", -32600, "The request is not a valid JSON-RPC 2.0 request object."},
	{"methodNotFound", -32601, "The method does not exist."},
	{"invalidParams", -32602, "The params are missing, unknown or invalid."},
	{"internal", -32603, "The server failed; the message says why."},
}

// Schema is every file, validated and cross-referenced.
type Schema struct {
	Protocol int
	Domains  []*File // sorted by domain name
	Errors   []ErrorDef

	enums map[string]*Enum
	types map[string]*Type
}

// TypeRef is a parsed type expression: a primitive, a named enum or type,
// []T or map[string]T.
type TypeRef struct {
	Prim  string // bool, int, float, string, time, json
	Named string
	List  *TypeRef
	Map   *TypeRef
}

var prims = []string{"bool", "int", "float", "string", "time", "json"}

func parseTypeRef(s string) (*TypeRef, error) {
	switch {
	case strings.HasPrefix(s, "[]"):
		elem, err := parseTypeRef(s[2:])
		if err != nil {
			return nil, err
		}
		return &TypeRef{List: elem}, nil
	case strings.HasPrefix(s, "map[string]"):
		elem, err := parseTypeRef(s[len("map[string]"):])
		if err != nil {
			return nil, err
		}
		return &TypeRef{Map: elem}, nil
	case slices.Contains(prims, s):
		return &TypeRef{Prim: s}, nil
	case pascalRE.MatchString(s):
		return &TypeRef{Named: s}, nil
	}
	return nil, fmt.Errorf("invalid type expression %q", s)
}

// String is the IDL spelling.
func (t *TypeRef) String() string {
	switch {
	case t.List != nil:
		return "[]" + t.List.String()
	case t.Map != nil:
		return "map[string]" + t.Map.String()
	case t.Named != "":
		return t.Named
	}
	return t.Prim
}

var (
	domainRE = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	camelRE  = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	pascalRE = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	valueRE  = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
)

// Load reads and validates every *.yaml file in dir.
func Load(dir string) (*Schema, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no schema files in %s: %w", dir, fs.ErrNotExist)
	}
	slices.Sort(paths)
	var files []*File
	for _, p := range paths {
		f, err := loadFile(p)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return build(files)
}

func loadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var f File
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f.path = path
	return &f, nil
}

// build validates the files together; every problem is reported at once.
func build(files []*File) (*Schema, error) {
	s := &Schema{enums: map[string]*Enum{}, types: map[string]*Type{}}
	var errs []error
	fail := func(f *File, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", f.path, fmt.Sprintf(format, args...)))
	}

	errorNames := map[string]bool{}
	errorCodes := map[int]string{}
	for _, e := range builtinErrors {
		errorNames[e.Name] = true
		errorCodes[e.Code] = e.Name
	}
	domains := map[string]bool{}
	global := map[string]string{} // Go-level names, to catch generated collisions
	for _, name := range reservedAPINames {
		global[name] = "a hand-written identifier in package api"
	}

	claim := func(f *File, name, what string) {
		if prev, ok := global[name]; ok {
			fail(f, "%s %s collides with %s", what, name, prev)
			return
		}
		global[name] = what + " in " + f.path
	}

	for _, f := range files {
		if len(f.Errors) > 0 {
			if f.Domain != "" || len(f.Enums)+len(f.Types)+len(f.Methods)+len(f.Events) > 0 {
				fail(f, "a file with errors must declare nothing else")
			}
			for _, e := range f.Errors {
				switch {
				case !camelRE.MatchString(e.Name):
					fail(f, "error name %q must be lowerCamel", e.Name)
				case errorNames[e.Name]:
					fail(f, "duplicate error %q", e.Name)
				case e.Code <= 0:
					fail(f, "error %s: application codes are positive", e.Name)
				case errorCodes[e.Code] != "":
					fail(f, "error %s reuses code %d of %s", e.Name, e.Code, errorCodes[e.Code])
				}
				errorNames[e.Name] = true
				errorCodes[e.Code] = e.Name
				s.Errors = append(s.Errors, e)
			}
			continue
		}
		if !domainRE.MatchString(f.Domain) {
			fail(f, "domain %q must be lowercase letters and digits", f.Domain)
			continue
		}
		if domains[f.Domain] {
			fail(f, "duplicate domain %q", f.Domain)
			continue
		}
		domains[f.Domain] = true
		if reservedClientMethods[goName(f.Domain)] {
			fail(f, "domain %q collides with the hand-written Client.%s", f.Domain, goName(f.Domain))
		}
		if f.Protocol != 0 {
			if s.Protocol != 0 {
				fail(f, "protocol is set in more than one file")
			}
			s.Protocol = f.Protocol
		}
		if len(f.Callers) == 0 {
			fail(f, "domain %q must list its callers (%s)", f.Domain, strings.Join(callerKinds, ", "))
		}
		seenCaller := map[string]bool{}
		for _, c := range f.Callers {
			if !slices.Contains(callerKinds, c) || seenCaller[c] {
				fail(f, "domain %q: caller %q must be one of %s, listed once", f.Domain, c, strings.Join(callerKinds, ", "))
			}
			seenCaller[c] = true
		}
		s.Domains = append(s.Domains, f)
		claim(f, goName(f.Domain)+"Service", "service")
		claim(f, goName(f.Domain)+"Client", "client")
		for i := range f.Enums {
			e := &f.Enums[i]
			if !pascalRE.MatchString(e.Name) {
				fail(f, "enum %q must be PascalCase", e.Name)
				continue
			}
			claim(f, e.Name, "enum")
			s.enums[e.Name] = e
			if len(e.Values) == 0 {
				fail(f, "enum %s has no values", e.Name)
			}
			seen := map[string]bool{}
			for _, v := range e.Values {
				if !valueRE.MatchString(v.Name) || seen[v.Name] {
					fail(f, "enum %s: value %q must be unique lowercase letters and digits", e.Name, v.Name)
				}
				seen[v.Name] = true
				claim(f, e.Name+goName(v.Name), "enum value")
			}
		}
		for i := range f.Types {
			t := &f.Types[i]
			if !pascalRE.MatchString(t.Name) {
				fail(f, "type %q must be PascalCase", t.Name)
				continue
			}
			claim(f, t.Name, "type")
			s.types[t.Name] = t
		}
		for _, m := range f.Methods {
			if !camelRE.MatchString(m.Name) {
				fail(f, "method %q must be lowerCamel", m.Name)
				continue
			}
			claim(f, paramsName(f.Domain, m.Name), "params type")
			seen := map[string]bool{}
			for _, c := range m.Callers {
				if !slices.Contains(f.Callers, c) || seen[c] {
					fail(f, "%s.%s: caller %q must be one of the domain's (%s), listed once", f.Domain, m.Name, c, strings.Join(f.Callers, ", "))
				}
				seen[c] = true
			}
		}
		for _, ev := range f.Events {
			if !camelRE.MatchString(ev.Name) {
				fail(f, "event %q must be lowerCamel", ev.Name)
				continue
			}
			claim(f, eventName(f.Domain, ev.Name), "event type")
		}
	}
	if s.Protocol == 0 {
		errs = append(errs, errors.New("no file sets protocol"))
	}

	resolve := func(f *File, where, expr string) *TypeRef {
		ref, err := parseTypeRef(expr)
		if err != nil {
			fail(f, "%s: %v", where, err)
			return nil
		}
		for r := ref; r != nil; r = cmpNonNil(r.List, r.Map) {
			if r.Named != "" && s.enums[r.Named] == nil && s.types[r.Named] == nil {
				fail(f, "%s: unknown type %s", where, r.Named)
				return nil
			}
		}
		return ref
	}
	fields := func(f *File, where string, fs []Field) {
		seen := map[string]bool{}
		for i := range fs {
			fd := &fs[i]
			if !camelRE.MatchString(fd.Name) || seen[fd.Name] {
				fail(f, "%s: field %q must be unique lowerCamel", where, fd.Name)
			}
			seen[fd.Name] = true
			fd.ref = resolve(f, where+"."+fd.Name, fd.Type)
		}
	}
	for _, f := range s.Domains {
		for i := range f.Types {
			fields(f, f.Types[i].Name, f.Types[i].Fields)
		}
		for i := range f.Methods {
			m := &f.Methods[i]
			where := f.Domain + "." + m.Name
			fields(f, where, m.Params)
			if m.Result != "" {
				m.result = resolve(f, where+" result", m.Result)
			}
			for _, e := range m.Errors {
				if !errorNames[e] {
					fail(f, "%s: unknown error %q", where, e)
				}
			}
			if slices.Contains(m.admits(f), "agent") && f.Domain != unscopedDomain && m.scopeField() == nil {
				fail(f, "%s admits agent, so it must name its vault in a required int %s", where, scopeParam)
			}
		}
		for i := range f.Events {
			fields(f, f.Domain+"."+f.Events[i].Name, f.Events[i].Fields)
		}
	}
	slices.SortFunc(s.Domains, func(a, b *File) int { return strings.Compare(a.Domain, b.Domain) })
	return s, errors.Join(errs...)
}

// scopeField is the method's vault param, a required int vaultId, or nil.
func (m Method) scopeField() *Field {
	for i := range m.Params {
		p := &m.Params[i]
		if p.Name == scopeParam && !p.Optional && p.ref != nil && p.ref.Prim == "int" {
			return p
		}
	}
	return nil
}

func cmpNonNil(a, b *TypeRef) *TypeRef {
	if a != nil {
		return a
	}
	return b
}

// isEnum reports whether a named type is an enum.
func (s *Schema) isEnum(name string) bool { return s.enums[name] != nil }

// allErrors is the builtin codes followed by the application codes.
func (s *Schema) allErrors() []ErrorDef { return append(slices.Clone(builtinErrors), s.Errors...) }

// initialisms are spelled in capitals in Go names, per Go convention.
var initialisms = map[string]string{
	"api": "API", "fts": "FTS", "html": "HTML", "http": "HTTP", "id": "ID", "ids": "IDs",
	"json": "JSON", "mcp": "MCP", "rpc": "RPC", "tls": "TLS", "uid": "UID", "url": "URL", "urls": "URLs",
	"yaml": "YAML",
}

// goName turns a lowerCamel or lowercase IDL name into an exported Go name.
func goName(s string) string {
	var b strings.Builder
	for _, w := range splitWords(s) {
		if v, ok := initialisms[strings.ToLower(w)]; ok {
			b.WriteString(v)
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}

func splitWords(s string) []string {
	var words []string
	start := 0
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			words = append(words, s[start:i])
			start = i
		}
	}
	return append(words, s[start:])
}

func paramsName(domain, method string) string { return goName(domain) + goName(method) + "Params" }
func eventName(domain, event string) string   { return goName(domain) + goName(event) }
func wireMethod(domain, method string) string { return domain + "." + method }
