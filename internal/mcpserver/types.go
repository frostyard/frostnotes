package mcpserver

// The tools' inputs and outputs (docs/specs/mcp.md). The SDK infers each
// tool's JSON schemas from them: a field without omitzero is required,
// and a jsonschema tag is its description. The SDK encodes them with
// encoding/json, so times are RFC 3339 strings.

type searchIn struct {
	Query string `json:"query" jsonschema:"the words to find; each matches as a prefix, and a phrase in double quotes as written"`
	Limit int    `json:"limit,omitzero" jsonschema:"at most this many hits, 1 to 100; default 20"`
}

type searchHit struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Snippet string `json:"snippet" jsonschema:"words around the best match, the matching ones between ** and **"`
}

type searchOut struct {
	Hits   []searchHit `json:"hits"`
	Notice string      `json:"notice"`
}

type readIn struct {
	Path string `json:"path" jsonschema:"the note's path in the vault, such as Projects/Plan.md"`
}

type noteOut struct {
	Path       string         `json:"path"`
	Title      string         `json:"title"`
	Hash       string         `json:"hash" jsonschema:"the version read: what a write that started from it names as its base"`
	Modified   string         `json:"modified" jsonschema:"when the file last changed, RFC 3339 in UTC"`
	Properties map[string]any `json:"properties" jsonschema:"the YAML frontmatter as an object; empty without one"`
	Tags       []string       `json:"tags"`
	Aliases    []string       `json:"aliases"`
	Content    string         `json:"content" jsonschema:"the note's markdown, frontmatter included"`
	Notice     string         `json:"notice"`
}

type listIn struct {
	Folder   string `json:"folder,omitzero" jsonschema:"only notes in this folder or below it; / is the whole vault"`
	Tag      string `json:"tag,omitzero" jsonschema:"only notes with this tag or one nested under it, without #"`
	Property string `json:"property,omitzero" jsonschema:"only notes whose frontmatter has this key"`
	Value    string `json:"value,omitzero" jsonschema:"with property: only notes where it has this value, ignoring case"`
	Limit    int    `json:"limit,omitzero" jsonschema:"at most this many notes, 1 to 1000; default 200"`
}

type listEntry struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Modified string `json:"modified" jsonschema:"RFC 3339 in UTC"`
}

type listOut struct {
	Notes  []listEntry `json:"notes"`
	Total  int         `json:"total" jsonschema:"how many notes match, beyond limit included"`
	Notice string      `json:"notice"`
}

type backlinksIn struct {
	Path string `json:"path" jsonschema:"the path of the note or attachment linked to"`
}

type backlink struct {
	Source string `json:"source" jsonschema:"the linking note's path"`
	Line   int    `json:"line"`
	Target string `json:"target" jsonschema:"the link's target as written"`
	Alias  string `json:"alias"`
	Embed  bool   `json:"embed"`
}

type backlinksOut struct {
	Backlinks []backlink `json:"backlinks"`
	Notice    string     `json:"notice"`
}

type createIn struct {
	Path    string `json:"path" jsonschema:"the new note's path in the vault, ending in .md"`
	Content string `json:"content" jsonschema:"the note's markdown, with frontmatter if it should have properties"`
}

type createOut struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

type editIn struct {
	Path       string         `json:"path" jsonschema:"the note's path in the vault"`
	OldText    string         `json:"old_text,omitzero" jsonschema:"text that occurs exactly once in the note, to replace with new_text"`
	NewText    string         `json:"new_text,omitzero" jsonschema:"with old_text: what replaces it; empty deletes it"`
	Append     string         `json:"append,omitzero" jsonschema:"text to add at the end of the note"`
	Prepend    string         `json:"prepend,omitzero" jsonschema:"text to add at the start of the note, after its properties"`
	Properties map[string]any `json:"properties,omitzero" jsonschema:"frontmatter keys to set; null removes a key"`
}

type editOut struct {
	Path      string `json:"path"`
	Hash      string `json:"hash"`
	Merged    bool   `json:"merged" jsonschema:"the note had changed since it was read, and the edit was merged into it"`
	Conflicts int    `json:"conflicts" jsonschema:"passages changed both ways, where the other change stood"`
}

type dailyIn struct {
	Text string `json:"text" jsonschema:"the text to append"`
	Date string `json:"date,omitzero" jsonschema:"the day, YYYY-MM-DD; default today"`
}

type dailyOut struct {
	Path    string `json:"path"`
	Hash    string `json:"hash"`
	Created bool   `json:"created" jsonschema:"the daily note was created for this call"`
	Merged  bool   `json:"merged"`
}
