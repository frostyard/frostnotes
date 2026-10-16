// Package vaultgen generates deterministic, fictional markdown vaults and link manifests.
package vaultgen

import (
	"bytes"
	"cmp"
	"fmt"
	"image"
	"image/png"
	"math/rand/v2"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Options controls the size and randomness of a generated vault.
type Options struct {
	Seed  uint64
	Notes int // Number of markdown files; at least 20.
}

// Manifest lists every file, real link and code-hidden decoy in a vault.
type Manifest struct {
	Seed        uint64   `json:"seed"`
	Notes       []string `json:"notes"`
	Attachments []string `json:"attachments"`
	Links       []Link   `json:"links"`
	Decoys      []Link   `json:"decoys"`
}

// Link records exact markup, its byte offset and its expected resolution.
type Link struct {
	Source string `json:"source"`
	Text   string `json:"text"`
	Start  int    `json:"start"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Want   string `json:"want,omitzero"`
}

type noteSpec struct {
	name, date, previous string
	daily                bool
}

type generator struct {
	rng    *rand.Rand
	m      Manifest
	names  map[string]int
	dirs   map[string]bool
	images map[string]string
	words  []string
}

// Generate writes a vault of o.Notes notes into dir and returns its manifest.
// The destination must be missing or an empty directory.
func Generate(dir string, o Options) (Manifest, error) {
	if o.Notes < 20 {
		return Manifest{}, fmt.Errorf("generate vault: need at least 20 notes")
	}
	root, err := emptyRoot(dir)
	if err != nil {
		return Manifest{}, err
	}
	defer root.Close()
	g := generator{
		rng: rand.New(rand.NewPCG(o.Seed, o.Seed^0x9e3779b97f4a7c15)),
		m: Manifest{Seed: o.Seed, Attachments: []string{
			"Garden/media/img-1.png", "Orchard/media/img-2.png",
		}},
		names: make(map[string]int), dirs: make(map[string]bool),
		images: make(map[string]string), words: strings.Fields(proseWords),
	}
	notes := g.layout(o.Notes)
	slices.Sort(g.m.Attachments)
	for _, attachment := range g.m.Attachments {
		g.images[path.Dir(path.Dir(attachment))+"/Overview.md"] = attachment
	}
	for _, n := range notes {
		g.m.Notes = append(g.m.Notes, n.name)
		g.names[strings.ToLower(path.Base(n.name))]++
	}
	slices.Sort(g.m.Notes)
	if err := g.attachments(root); err != nil {
		return Manifest{}, err
	}
	// Inbox comes last so its reading list can make up any random link deficit.
	for _, n := range slices.Concat(notes[1:], notes[:1]) {
		body := g.render(n)
		if err := g.write(root, n.name, []byte(body)); err != nil {
			return Manifest{}, err
		}
	}
	slices.SortFunc(g.m.Links, compareLinks)
	slices.SortFunc(g.m.Decoys, compareLinks)
	return g.m, nil
}

func emptyRoot(dir string) (*os.Root, error) {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		if err := os.Mkdir(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create vault: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("inspect vault: %w", err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("generate vault: destination is not a directory")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open vault: %w", err)
	}
	f, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, fmt.Errorf("open vault directory: %w", err)
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil || len(entries) != 0 {
		root.Close()
		if err != nil {
			return nil, fmt.Errorf("read vault directory: %w", err)
		}
		return nil, fmt.Errorf("generate vault: destination is not empty")
	}
	return root, nil
}

func (g *generator) layout(total int) []noteSpec {
	notes := []noteSpec{{name: "Inbox.md", date: "2026-10-16"}}
	end := time.Date(2026, time.October, 16, 0, 0, 0, 0, time.UTC)
	for i := min(90, max(2, total/10)) - 1; i >= 0; i-- {
		date := end.AddDate(0, 0, -i).Format(time.DateOnly)
		notes = append(notes, noteSpec{name: date + ".md", date: date, daily: true})
	}
	start := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	for _, member := range []string{"Wren", "Finch", "Lark"} {
		previous := ""
		// Cap meetings before the earliest daily note, even in large vaults.
		for i := range min(20, max(2, total/30)) {
			date := start.AddDate(0, 0, 7*i).Format(time.DateOnly)
			name := "Members/" + member + "/" + date + ".md"
			notes = append(notes, noteSpec{name: name, date: date, previous: previous})
			previous = name
		}
	}
	for _, name := range []string{
		"Members/Overview.md", "Garden/Overview.md", "Orchard/Overview.md",
		"Orchard/Plan.md", "Orchard/North Block/Plan.md",
		"Orchard/North Block/Seed Beds/Plan.md",
	} {
		notes = append(notes, noteSpec{name: name})
	}
	folders := []string{"Garden", "Orchard", "Orchard/North Block", "Orchard/North Block/Seed Beds", "Members"}
	folders, overviews := g.topicFolders(folders, total)
	notes = append(notes, overviews...)
	words := []string{"Café ledger", "Quince harvest", "Apple baskets", "Pear grafts", "Plum nursery"}
	for i := 0; len(notes) < total; i++ {
		folder, word := folders[i%len(folders)], words[i%len(words)]
		if i >= len(folders) {
			folder, word = folders[g.rng.IntN(len(folders))], words[g.rng.IntN(len(words))]
		}
		notes = append(notes, noteSpec{name: fmt.Sprintf("%s/%s %d.md", folder, word, i+1)})
	}
	return notes
}

func (g *generator) topicFolders(folders []string, total int) ([]string, []noteSpec) {
	first := strings.Fields("Upper Lower Stone Mossy Sunny Shady Willow Hazel")
	second := strings.Fields("Meadow Cellar Terrace Grove Clearing Field Shed Hollow")
	// Only folders with room for a child are eligible parents.
	parents := []string{"Garden", "Orchard", "Orchard/North Block"}
	var overviews []noteSpec
	pairs := len(first) * len(second)
	for i := range max(4, total/9) - 4 {
		folder := first[i/len(second)%len(first)] + " " + second[i%len(second)]
		if i >= pairs {
			folder += fmt.Sprintf(" %d", i/pairs+1)
		}
		if i%3 == 0 {
			overviews = append(overviews, noteSpec{name: folder + "/Overview.md"})
			if i%9 == 0 {
				g.m.Attachments = append(g.m.Attachments, fmt.Sprintf("%s/media/img-%d.png", folder, len(g.m.Attachments)+1))
			}
		} else {
			folder = parents[g.rng.IntN(len(parents))] + "/" + folder
		}
		folders = append(folders, folder)
		if strings.Count(folder, "/") < 2 {
			parents = append(parents, folder)
		}
	}
	return folders, overviews
}

// Vocabulary is fictional orchard and co-op terminology, with no link syntax.
const proseWords = `orchard apple pear quince plum cherry peach apricot fig berry
harvest blossom branch trunk root leaf bud shoot seed seedling sapling graft
rootstock scion pruning canopy trellis stake twine ladder basket crate barrel
bucket shovel spade rake hoe fork trowel shears gloves boots apron wheelbarrow
compost mulch humus loam clay sand soil bed furrow row terrace slope meadow
pasture grove copse hedge hedgerow fence gate path track lane ditch drain pond
stream well spring rain frost snow mist dew cloud breeze sunshine shade warmth
season winter autumn summer thaw drought watering irrigation moisture runoff
pollinator bee butterfly moth beetle worm bird robin wren finch lark nest hive
nectar pollen petal flower clover vetch grass nettle comfrey fern moss willow
hazel oak ash alder birch maple elder walnut chestnut acorn nut kernel shell
fruit skin flesh stone pip juice cider press pulp pomace jam jelly preserve
drying storage cellar pantry shelf rack jar lid label sack weighing sorting
grading packing delivery cart stall market share member helper neighbor crew
team steward grower gardener volunteer gathering meeting agenda minutes vote
budget ledger receipt invoice dues fund grant cost price order stock surplus
exchange donation kitchen table bench workshop repair tool handle blade hinge
roof beam board nail screw oil canvas rope tarp shelter shed barn nursery
greenhouse coldframe germination planting spacing thinning feeding tending
inspection survey count sample trial record sketch map plan goal task rota
schedule morning evening weekend fortnight month year early late fresh ripe
green golden russet sweet tart crisp soft sturdy gentle quiet careful shared
local common weekly annual useful ready steady abundant and the with beside
through around beneath toward after before during from for in our we gather
carry check tend note measure mend prepare store collect watch choose plant`

func (g *generator) paragraphs(b *strings.Builder, count int) {
	for range count {
		for sentence := range 3 + g.rng.IntN(5) {
			if sentence > 0 {
				b.WriteByte(' ')
			}
			for word := range 8 + g.rng.IntN(13) {
				text := g.words[g.rng.IntN(len(g.words))]
				if word == 0 {
					b.WriteString(strings.ToUpper(text[:1]))
					b.WriteString(text[1:])
				} else {
					b.WriteByte(' ')
					b.WriteString(text)
				}
			}
			b.WriteByte('.')
		}
		if g.rng.IntN(4) == 0 {
			b.WriteString(" #orchard/growing")
		}
		b.WriteString("\n\n")
	}
}

func (g *generator) render(n noteSpec) string {
	var b strings.Builder
	title := strings.TrimSuffix(path.Base(n.name), ".md")
	if n.date != "" || g.rng.IntN(100) < 85 {
		date := n.date
		if date == "" {
			date = "2026-10-16"
		}
		fmt.Fprintf(&b, "---\ntitle: %q\ncreated: %s\ntags: [orchard, co-op]\n", title, date)
		if !n.daily && g.rng.IntN(4) == 0 {
			fmt.Fprintf(&b, "aliases: [%q]\n", "Co-op "+title)
		}
		b.WriteString("---\n")
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	if n.daily || n.name == "Inbox.md" {
		b.WriteString("The orchard beds hold quince, pears and apple seedlings. #orchard/growing\n\n")
	} else if n.date != "" {
		g.paragraphs(&b, 1+g.rng.IntN(4))
	} else {
		g.paragraphs(&b, 1)
	}
	b.WriteString("- [ ] Water the seedlings\n- [x] Sort the baskets\n\n")
	switch {
	case n.daily:
		b.WriteString("## Log\n")
		if g.rng.IntN(2) == 0 {
			g.paragraphs(&b, 1+g.rng.IntN(3))
		}
	case n.name == "Inbox.md":
		g.inbox(&b, n.name)
	case n.date != "":
		if n.previous != "" {
			g.wiki(&b, n.name, n.previous, "")
		}
	default:
		g.topic(&b, n.name)
	}
	body := b.String()
	if g.rng.IntN(2) == 0 {
		body = strings.TrimRight(body, "\n")
	}
	return body
}

func (g *generator) topic(b *strings.Builder, source string) {
	b.WriteString("## Goals\n\n")
	paragraphs := 3 + g.rng.IntN(10)
	if g.rng.IntN(40) == 0 {
		paragraphs = 30 + g.rng.IntN(10)
	}
	g.paragraphs(b, paragraphs)
	for range g.rng.IntN(4) {
		g.wiki(b, source, g.m.Notes[g.rng.IntN(len(g.m.Notes))], "")
	}
	if source == "Orchard/Overview.md" {
		g.wiki(b, source, "Orchard/Plan.md", "")
	}
	if attachment, ok := g.images[source]; ok {
		g.markdown(b, source, "./media/"+path.Base(attachment), attachment, true)
	}
	if g.rng.IntN(5) == 0 {
		b.WriteString("\n> [!important] Harvest\n> Leave some fruit for the birds.\n\n")
	}
	if g.rng.IntN(5) == 0 {
		g.decoy(b, source, g.rng.IntN(2) == 0)
	}
}

func (g *generator) inbox(b *strings.Builder, source string) {
	b.WriteString("> [!important] Co-op reading\n> Bring these notes to the next gathering.\n\n")
	g.wiki(b, source, "Garden/Café ledger 1.md", "")
	g.wiki(b, source, "Orchard/North Block/Plan.md", "")
	g.wiki(b, source, "Garden/Overview.md", "#Goals")
	g.wiki(b, source, "Members/Wren/2026-01-05.md", "|last meeting")
	g.record(b, Link{Source: source, Text: "[[Someday quince 17]]", Kind: "wiki", Target: "Someday quince 17"}, false)
	g.markdown(b, source, "Garden/Café ledger 1.md", "Garden/Café ledger 1.md", false)
	// Root media folders are forbidden; use a relative path into Garden instead.
	g.markdown(b, source, "./Garden/media/img-1.png", "Garden/media/img-1.png", true)
	g.record(b, Link{Source: source, Text: "![[img-1.png]]", Kind: "wiki", Target: "img-1.png", Want: "Garden/media/img-1.png"}, false)
	g.decoy(b, source, true)
	g.decoy(b, source, false)
	for i := 0; len(g.m.Links) <= len(g.m.Notes); i++ {
		g.wiki(b, source, g.m.Notes[i%len(g.m.Notes)], "")
	}
}

func (g *generator) wiki(b *strings.Builder, source, want, suffix string) {
	target := strings.TrimSuffix(want, ".md")
	if path.Dir(source) == path.Dir(want) || g.names[strings.ToLower(path.Base(want))] == 1 {
		target = path.Base(target)
	}
	g.record(b, Link{Source: source, Text: "[[" + target + suffix + "]]", Kind: "wiki", Target: target, Want: want}, false)
}

func (g *generator) markdown(b *strings.Builder, source, target, want string, embed bool) {
	segments := strings.Split(target, "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	text := "[the plan](" + strings.Join(segments, "/") + ")"
	if embed {
		text = "![a map](" + strings.Join(segments, "/") + ")"
	}
	g.record(b, Link{Source: source, Text: text, Kind: "markdown", Target: target, Want: want}, false)
}

func (g *generator) decoy(b *strings.Builder, source string, fenced bool) {
	prefix, suffix := "\nUse `", "` as an example.\n"
	if fenced {
		prefix, suffix = "\n```\n", "\n```\n"
	}
	b.WriteString(prefix)
	target := fmt.Sprintf("Decoy %d", len(g.m.Decoys)+1)
	g.record(b, Link{Source: source, Text: "[[" + target + "]]", Kind: "wiki", Target: target}, true)
	b.WriteString(suffix)
}

func (g *generator) record(b *strings.Builder, link Link, decoy bool) {
	link.Start = b.Len()
	b.WriteString(link.Text)
	if decoy {
		g.m.Decoys = append(g.m.Decoys, link)
	} else {
		g.m.Links = append(g.m.Links, link)
		b.WriteString("\n\n")
	}
}

func compareLinks(a, b Link) int {
	if c := strings.Compare(a.Source, b.Source); c != 0 {
		return c
	}
	return cmp.Compare(a.Start, b.Start)
}

func (g *generator) attachments(root *os.Root) error {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		return fmt.Errorf("encode attachment: %w", err)
	}
	for _, name := range g.m.Attachments {
		if err := g.write(root, name, data.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) write(root *os.Root, name string, data []byte) error {
	dir := path.Dir(name)
	if !g.dirs[dir] {
		if err := root.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
			return fmt.Errorf("create folder %s: %w", dir, err)
		}
		g.dirs[dir] = true
	}
	if err := root.WriteFile(filepath.FromSlash(name), data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
