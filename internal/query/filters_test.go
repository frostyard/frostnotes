package query_test

import (
	"slices"
	"testing"
)

// allFiles is every file of the fixture vault, by path.
var allFiles = []string{
	"Areas/Garden.md", "Areas/Work.md", "Attachments/Doc.pdf", "Attachments/data.txt", "Attachments/pic.png",
	"Broken.md", "Daily/2026-03-05.md", "Daily/2026-03-06.md", "Dashboards/Open.base", "Home.md", "Inbox.md",
	"Misc/Many.md", "Misc/Spaced Out.md", "Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md",
	"Projects/Delta.md", "Projects/Gamma.md", "Reading/Article One.md", "Reading/Books.md", "Unicode Ünï.md",
}

// except is allFiles without the named paths.
func except(names ...string) []string {
	var out []string
	for _, p := range allFiles {
		if !slices.Contains(names, p) {
			out = append(out, p)
		}
	}
	return out
}

func TestFilterStatements(t *testing.T) {
	e := fixtureEnv(t)
	tests := []struct {
		name string
		stmt string
		want []string
	}{
		// Equality and the forms of a reference.
		{"bare key", `status == "active"`, []string{"Inbox.md", "Projects/Alpha.md"}},
		{"note.key", `note.status == 'active'`, []string{"Inbox.md", "Projects/Alpha.md"}},
		{"note[key], case counts", `note["Status"] == "active"`, []string{"Misc/Spaced Out.md"}},
		{"bare key, case counts", `Status == "active"`, []string{"Misc/Spaced Out.md"}},
		{"text is exact, case kept", `status == "Active"`, []string{"Unicode Ünï.md"}},
		{"!= keeps the missing", `status != "done"`, except("Projects/Archive/Old.md", "Projects/Beta.md")},
		{"! of ==", `!(status == "done")`, except("Projects/Archive/Old.md", "Projects/Beta.md")},
		{"bare reference: a string that is not empty", `status`,
			[]string{"Inbox.md", "Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Gamma.md", "Unicode Ünï.md"}},
		{"! of a bare reference", `!status`,
			except("Inbox.md", "Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Gamma.md", "Unicode Ünï.md")},
		{"an empty string is false", `note.note`, nil},
		{"a number other than 0 is true", `mood`, []string{"Areas/Garden.md", "Areas/Work.md", "Daily/2026-03-05.md", "Daily/2026-03-06.md", "Misc/Many.md"}},
		{"a list that is not empty is true", `authors`, []string{"Reading/Books.md"}},
		{"an empty list is false", `items`, nil},
		{"a null is false", `notes`, nil},

		// Numbers, untyped: the literal sets the comparison.
		{"> on numbers", `rating > 3`, []string{"Areas/Garden.md", "Misc/Many.md", "Projects/Alpha.md", "Reading/Article One.md"}},
		{">= on numbers", `rating >= 3`, []string{"Areas/Garden.md", "Misc/Many.md", "Projects/Alpha.md", "Projects/Beta.md", "Reading/Article One.md", "Reading/Books.md"}},
		{"< on numbers", `rating < 3`, []string{"Areas/Work.md", "Inbox.md", "Misc/Spaced Out.md", "Projects/Archive/Old.md"}},
		{"<= on numbers", `rating <= 2`, []string{"Areas/Work.md", "Inbox.md", "Misc/Spaced Out.md", "Projects/Archive/Old.md"}},
		{"== on numbers", `rating == 3`, []string{"Projects/Beta.md", "Reading/Books.md"}},
		{"== on a real that is whole", `rating == 3.0`, []string{"Projects/Beta.md", "Reading/Books.md"}},
		{"== on a fraction", `rating == 4.5`, []string{"Projects/Alpha.md"}},
		{"!= on numbers", `rating != 3`, except("Projects/Beta.md", "Reading/Books.md")},
		{"literal on the left", `5 < rating`, []string{"Misc/Many.md"}},
		{"== with the literal on the left", `5 == rating`, []string{"Areas/Garden.md", "Reading/Article One.md"}},
		{"a number literal with an exponent", `rating >= 1e1`, []string{"Misc/Many.md"}},
		{"a signed number", `rating > -1 && rating < 2`, []string{"Projects/Archive/Old.md"}},
		{"text never converts to a number", `rating == "5"`, nil},
		{"a number never equals text", `rating == "high"`, []string{"Projects/Gamma.md"}},
		{"strings order by code point", `rating > "a"`, []string{"Projects/Gamma.md"}},
		{"< on text", `rating < "z"`, []string{"Projects/Gamma.md"}},
		{"a text never orders against a number", `rating >= 0 && rating == "high"`, nil},

		// Types from the vault's settings.
		{"typed number: text does not convert", `priority == 5`, nil},
		{"typed number: != is true for it", `priority != 5`, allFiles},
		{"typed number <", `priority < 3`, []string{"Projects/Alpha.md", "Projects/Beta.md"}},
		{"typed number >=", `priority >= 3`, []string{"Misc/Many.md", "Projects/Archive/Old.md"}},
		{"typed checkbox: bare", `done`, []string{"Misc/Many.md", "Projects/Beta.md", "Projects/Gamma.md"}},
		{"typed checkbox == true", `done == true`, []string{"Misc/Many.md", "Projects/Beta.md"}},
		{"typed checkbox == false", `done == false`, []string{"Projects/Alpha.md"}},
		{"typed checkbox != true", `done != true`, except("Misc/Many.md", "Projects/Beta.md")},
		{"literal true on the left", `true == done`, []string{"Misc/Many.md", "Projects/Beta.md"}},
		{"! of a checkbox", `!done`, except("Misc/Many.md", "Projects/Beta.md", "Projects/Gamma.md")},
		{"typed text", `owner == "Ada"`, []string{"Projects/Alpha.md"}},
		{"typed text: the key case counts", `Owner == "Shadow"`, []string{"Projects/Alpha.md"}},
		{"types are looked up lower-cased", `count == 12`, []string{"Reading/Books.md"}},
		{"typed number: text 12 is not 12", `count > 0`, []string{"Reading/Books.md"}},
		{"untyped text 12", `rating == 5 && kind == "article"`, []string{"Reading/Article One.md"}},

		// Booleans.
		{"a boolean condition == true", `file.hasTag("daily") == true`, []string{"Daily/2026-03-05.md", "Daily/2026-03-06.md"}},
		{"a boolean condition == false", `(file.ext == "md") == false`, []string{"Attachments/Doc.pdf", "Attachments/data.txt", "Attachments/pic.png", "Dashboards/Open.base"}},
		{"a boolean condition != true", `file.hasTag("daily") != true`, except("Daily/2026-03-05.md", "Daily/2026-03-06.md")},
		{"the literal true", `true`, allFiles},
		{"the literal false", `false`, nil},
		{"a string literal", `"x"`, allFiles},
		{"an empty string literal", `""`, nil},
		{"a number literal", `0`, nil},
		{"constants fold", `1 < 2`, allFiles},
		{"constants fold to false", `"a" == "b"`, nil},
		{"constants of two kinds", `1 == "1"`, nil},
		{"!= of constants of two kinds", `1 != "1"`, allFiles},

		// Dates, with the clock at 2026-03-05 15:00 in New York.
		{"typed date < today()", `due < today()`, []string{"Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md"}},
		{"typed date >= today()", `due >= today()`, []string{"Projects/Gamma.md"}},
		{"typed date == date()", `due == date("2026-03-01")`, []string{"Projects/Alpha.md"}},
		{"typed date against a text", `due > "2026-03-01"`, []string{"Projects/Gamma.md"}},
		{"typed date and a duration", `due < now() - "1 week"`, []string{"Projects/Archive/Old.md", "Projects/Beta.md"}},
		{"typed date and a month", `due > today() - "1 month"`, []string{"Projects/Alpha.md", "Projects/Beta.md", "Projects/Gamma.md"}},
		{"a month later", `due > today() + "1M"`, nil},
		{"a date plus a duration twice", `due > today() - "1 M" - "1w"`, []string{"Projects/Alpha.md", "Projects/Beta.md", "Projects/Gamma.md"}},
		{"datetime with an offset", `started < date("2026-01-01")`, []string{"Projects/Archive/Old.md"}},
		{"datetime without a zone", `started == date("2026-01-15T09:30")`, []string{"Projects/Alpha.md"}},
		{"datetime in New York", `started == date("2026-01-15 14:30:00Z")`, []string{"Projects/Alpha.md"}},
		{"untyped, with a date constant", `deadline >= date("2026-03-01T12:00:00Z")`, []string{"Projects/Alpha.md"}},
		{"a zone offset", `deadline == date("2026-03-01T07:00:00-05:00")`, []string{"Projects/Alpha.md"}},
		{"a date alone is local midnight", `deadline > date("2026-03-01")`, []string{"Projects/Alpha.md"}},
		{"a text that is not a date", `status > date("2020-01-01")`, nil},
		{"untyped dates: today() is midnight", `created <= today()`, []string{"Daily/2026-03-05.md"}},
		{"untyped dates: < today()", `created < today()`, nil},
		{"untyped dates: ==", `created == date("2026-03-05")`, []string{"Daily/2026-03-05.md"}},
		{"untyped dates: >=", `created >= date("2026-03-06")`, []string{"Daily/2026-03-06.md"}},
		{"now() is the time of the call", `created < now()`, []string{"Daily/2026-03-05.md"}},
		{"millisecond fractions", `deadline == date("2026-03-01T12:00:00.000Z")`, []string{"Projects/Alpha.md"}},
		{"date + hours", `created < date("2026-03-05") + "12 hours"`, []string{"Daily/2026-03-05.md"}},
		{"date + years", `due < date("2025-12-31") + "1 year"`, []string{"Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Gamma.md"}},
		{"duration units", `due > today() - "5 days" && due < today() - "1 d"`, []string{"Projects/Alpha.md"}},
		{"dates fold", `today() < now()`, allFiles},
		{"dates fold, false", `now() < today()`, nil},
		{"a date against a string folds false", `today() == "x"`, nil},
		{"file.mtime >", `file.mtime > date("2026-03-04")`, []string{"Projects/Alpha.md"}},
		{"file.mtime <", `file.mtime < date("2026-01-01")`, []string{"Projects/Archive/Old.md"}},
		{"file.mtime >=", `file.mtime >= date("2026-03-02T00:00:00Z")`, []string{"Home.md", "Projects/Alpha.md"}},
		{"file.mtime keeps milliseconds", `file.mtime == date("2026-03-04T12:00:00.123Z")`, []string{"Projects/Alpha.md"}},
		{"file.mtime against a text", `file.mtime < "2026-01-01"`, []string{"Projects/Archive/Old.md"}},

		// File properties.
		{"file.name", `file.name == "Alpha.md"`, []string{"Projects/Alpha.md"}},
		{"file.name keeps case", `file.name == "alpha.md"`, nil},
		{"file.name with a space and accents", `file.name == "Unicode Ünï.md"`, []string{"Unicode Ünï.md"}},
		{"file.basename", `file.basename == "Alpha"`, []string{"Projects/Alpha.md"}},
		{"file.basename of a dotted name", `file.basename == "Doc"`, []string{"Attachments/Doc.pdf"}},
		{"file.ext", `file.ext == "base"`, []string{"Dashboards/Open.base"}},
		{"file.ext of notes", `file.ext != "md"`, []string{"Attachments/Doc.pdf", "Attachments/data.txt", "Attachments/pic.png", "Dashboards/Open.base"}},
		{"file.ext is never empty here", `file.ext == ""`, nil},
		{"file.folder", `file.folder == "Projects"`, []string{"Projects/Alpha.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md"}},
		{"file.folder is empty at the root", `file.folder == ""`, []string{"Broken.md", "Home.md", "Inbox.md", "Unicode Ünï.md"}},
		{"file.folder, nested", `file.folder == "Projects/Archive"`, []string{"Projects/Archive/Old.md"}},
		{"file.path", `file.path == "Home.md"`, []string{"Home.md"}},
		{"file.path >", `file.path > "Reading"`, []string{"Reading/Article One.md", "Reading/Books.md", "Unicode Ünï.md"}},
		{"file.size <", `file.size < 10`, []string{"Attachments/data.txt"}},
		{"file.size ==", `file.size == 18`, []string{"Attachments/Doc.pdf"}},
		{"file.size !=", `file.size != 18`, except("Attachments/Doc.pdf")},
		{"a file property is a bare reference", `file.folder`, except("Broken.md", "Home.md", "Inbox.md", "Unicode Ünï.md")},

		// File functions.
		{"hasTag, nested tags", `file.hasTag("project")`, []string{"Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Delta.md"}},
		{"hasTag ignores case and #", `file.hasTag("#PROJECT")`, []string{"Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Delta.md"}},
		{"hasTag, a nested tag", `file.hasTag("area/work")`, []string{"Areas/Work.md", "Projects/Alpha.md", "Projects/Gamma.md"}},
		{"hasTag, a parent", `file.hasTag("area")`, []string{"Areas/Garden.md", "Areas/Work.md", "Projects/Alpha.md", "Projects/Gamma.md", "Reading/Article One.md"}},
		{"hasTag, several", `file.hasTag("daily", "index")`, []string{"Daily/2026-03-05.md", "Daily/2026-03-06.md", "Home.md"}},
		{"hasTag, accents", `file.hasTag("ÜNÏ")`, []string{"Unicode Ünï.md"}}, // the index folds with Go's ToLower: Ü is ü
		{"hasTag is not a prefix match", `file.hasTag("proj")`, nil},
		{"hasTag, an inline tag", `file.hasTag("todo")`, []string{"Inbox.md"}},
		{"inFolder", `file.inFolder("Projects")`, []string{"Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md"}},
		{"inFolder with a slash", `file.inFolder("/Projects/")`, []string{"Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md"}},
		{"inFolder, nested", `file.inFolder("Projects/Archive")`, []string{"Projects/Archive/Old.md"}},
		{"inFolder, the whole vault", `file.inFolder("/")`, allFiles},
		{"inFolder, empty", `file.inFolder("")`, allFiles},
		{"inFolder keeps case", `file.inFolder("projects")`, nil},
		{"inFolder is not a prefix match", `file.inFolder("Proj")`, nil},
		{"hasProperty", `file.hasProperty("status")`, []string{"Inbox.md", "Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Gamma.md", "Unicode Ünï.md"}},
		{"hasProperty, a null value", `file.hasProperty("notes")`, []string{"Areas/Garden.md"}},
		{"hasProperty keeps case", `file.hasProperty("Status")`, []string{"Misc/Spaced Out.md"}},
		{"hasLink, resolved", `file.hasLink("Alpha")`, []string{"Areas/Garden.md", "Broken.md", "Home.md", "Inbox.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md"}},
		{"hasLink, a path", `file.hasLink("Projects/Alpha")`, []string{"Areas/Garden.md", "Broken.md", "Home.md", "Inbox.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md"}},
		{"hasLink, a file with its extension", `file.hasLink("Reading/Books.md")`, []string{"Inbox.md", "Projects/Gamma.md", "Reading/Article One.md"}},
		{"hasLink, an attachment", `file.hasLink("pic.png")`, []string{"Reading/Article One.md"}},
		{"hasLink, unresolved", `file.hasLink("Nowhere")`, []string{"Inbox.md"}},
		{"hasLink, unresolved, ignoring case", `file.hasLink("nowhere")`, []string{"Inbox.md"}},
		{"hasLink to a note nobody wrote", `file.hasLink("Unwritten")`, []string{"Projects/Alpha.md"}},
		{"hasLink to a note nobody links", `file.hasLink("Home")`, nil},
		{"file.links.isEmpty()", `file.links.isEmpty()`, []string{"Areas/Work.md", "Attachments/Doc.pdf", "Attachments/data.txt", "Attachments/pic.png", "Daily/2026-03-05.md", "Daily/2026-03-06.md", "Dashboards/Open.base", "Misc/Many.md", "Misc/Spaced Out.md", "Reading/Books.md", "Unicode Ünï.md"}},
		{"file.links is a list: a bare reference", `file.links`, []string{"Areas/Garden.md", "Broken.md", "Home.md", "Inbox.md", "Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md", "Reading/Article One.md"}},

		// File tags as a list.
		{"file.tags.contains is exact", `file.tags.contains("project")`, []string{"Projects/Alpha.md", "Projects/Beta.md"}},
		{"file.tags.contains keeps the spelling", `file.tags.contains("Area/Home")`, []string{"Projects/Alpha.md"}},
		{"file.tags.contains, folded spelling", `file.tags.contains("area/home")`, []string{"Areas/Garden.md", "Reading/Article One.md"}},
		{"file.tags.containsAny", `file.tags.containsAny("daily", "inbox")`, []string{"Daily/2026-03-05.md", "Daily/2026-03-06.md", "Inbox.md"}},
		{"file.tags.containsAll", `file.tags.containsAll("project", "area/work")`, []string{"Projects/Alpha.md"}},
		{"file.tags.isEmpty()", `file.tags.isEmpty()`, []string{"Attachments/Doc.pdf", "Attachments/data.txt", "Attachments/pic.png", "Broken.md", "Dashboards/Open.base", "Misc/Many.md", "Misc/Spaced Out.md"}},
		{"file.tags: a bare reference", `file.tags`, except("Attachments/Doc.pdf", "Attachments/data.txt", "Attachments/pic.png", "Broken.md", "Dashboards/Open.base", "Misc/Many.md", "Misc/Spaced Out.md")},
		{"file.tags.startsWith is false on a list", `file.tags.startsWith("p")`, nil},
		{"file.tags.contains of a number", `file.tags.contains(5)`, nil},

		// Text methods.
		{"contains keeps case", `status.contains("act")`, []string{"Inbox.md", "Projects/Alpha.md"}},
		{"contains, capital", `status.contains("Act")`, []string{"Unicode Ünï.md"}},
		{"contains, empty", `status.contains("")`, []string{"Inbox.md", "Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Gamma.md", "Unicode Ünï.md"}},
		{"startsWith", `status.startsWith("ac")`, []string{"Inbox.md", "Projects/Alpha.md"}},
		{"startsWith keeps case", `status.startsWith("Ac")`, []string{"Unicode Ünï.md"}},
		{"endsWith", `status.endsWith("ve")`, []string{"Inbox.md", "Projects/Alpha.md", "Unicode Ünï.md"}},
		{"endsWith, longer than the text", `status.endsWith("an active")`, nil},
		{"endsWith, empty", `status.endsWith("")`, []string{"Inbox.md", "Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Gamma.md", "Unicode Ünï.md"}},
		{"typed text contains", `owner.contains("d")`, []string{"Projects/Alpha.md"}},
		{"text with an apostrophe", `status.contains("it's")`, nil},
		{"text with a percent sign", `status.contains("%")`, nil},
		{"text with an underscore", `status.contains("_")`, nil},
		{"file.name.contains", `file.name.contains("lph")`, []string{"Projects/Alpha.md"}},
		{"file.name.contains keeps case", `file.name.contains("LPH")`, nil},
		{"file.basename.startsWith", `file.basename.startsWith("Dai")`, nil},
		{"file.path.startsWith", `file.path.startsWith("Daily/")`, []string{"Daily/2026-03-05.md", "Daily/2026-03-06.md"}},
		{"file.path.endsWith", `file.path.endsWith("a.md")`, []string{"Projects/Alpha.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md"}},
		{"file.ext.contains", `file.ext.contains("as")`, []string{"Dashboards/Open.base"}},
		{"file.folder.contains", `file.folder.contains("Arch")`, []string{"Projects/Archive/Old.md"}},
		{"file.name.isEmpty()", `file.name.isEmpty()`, nil},
		{"file.folder.isEmpty()", `file.folder.isEmpty()`, []string{"Broken.md", "Home.md", "Inbox.md", "Unicode Ünï.md"}},
		{"file.name.containsAny is false on text", `file.name.containsAny("Alpha.md")`, nil},

		// Lists.
		{"list contains, typed multitext", `category.contains("core")`, []string{"Projects/Alpha.md"}},
		{"list contains, an item", `category.contains("alpha")`, []string{"Projects/Alpha.md"}},
		{"list contains is not a substring", `category.contains("alp")`, nil},
		{"list containsAny", `category.containsAny("beta", "core")`, []string{"Projects/Alpha.md", "Projects/Beta.md"}},
		{"list containsAll", `category.containsAll("alpha", "core")`, []string{"Projects/Alpha.md"}},
		{"list containsAll, one item missing", `category.containsAll("alpha", "beta")`, nil},
		{"typed list isEmpty: missing and []", `category.isEmpty()`, except("Projects/Alpha.md", "Projects/Beta.md")},
		{"untyped list contains", `authors.contains("Ada")`, []string{"Reading/Books.md"}},
		{"untyped list contains is exact", `authors.contains("A")`, nil},
		{"untyped list containsAny", `authors.containsAny("Zed", "Bob")`, []string{"Reading/Books.md"}},
		{"untyped: a text row is a substring", `kind.contains("ar")`, []string{"Areas/Garden.md", "Areas/Work.md", "Reading/Article One.md"}},
		{"untyped: a text row is not a list", `kind.containsAny("area")`, nil},
		{"untyped: a list row is not text", `authors.startsWith("A")`, nil},
		{"a list never equals a scalar", `authors == "Ada"`, nil},
		{"a list is != a scalar", `authors != "Ada"`, allFiles},
		// A list-typed property: a text value is a list of 1 item.
		{"typed list: a text value is 1 item", `tags.contains("area/work")`, []string{"Areas/Work.md", "Projects/Alpha.md", "Projects/Gamma.md"}},
		{"typed list: the item is not a substring", `tags.contains("area")`, nil},
		{"typed list: the item keeps its case", `tags.contains("Area/Work")`, nil},
		{"typed list: containsAny, an item or the text", `tags.containsAny("nope", "area/work")`, []string{"Areas/Work.md", "Projects/Alpha.md", "Projects/Gamma.md"}},
		{"typed list: containsAll, a text is 1 item", `tags.containsAll("area/work")`, []string{"Areas/Work.md", "Projects/Alpha.md", "Projects/Gamma.md"}},
		{"typed list: containsAll of 2 items is not a text", `tags.containsAll("area/work", "work")`, []string{"Areas/Work.md"}},
		{"typed list: containsAll of the same item twice", `tags.containsAll("area/work", "area/work")`, []string{"Areas/Work.md", "Projects/Alpha.md", "Projects/Gamma.md"}},
		{"typed aliases: a text value is 1 item", `aliases.contains("Gamma Prime")`, []string{"Projects/Gamma.md"}},
		{"typed aliases: the item is not a substring", `aliases.contains("Gamma")`, nil},
		{"typed aliases: an array item", `aliases.contains("Yard")`, []string{"Areas/Garden.md"}},
		{"typed aliases: containsAny", `aliases.containsAny("Yard", "Gamma Prime")`, []string{"Areas/Garden.md", "Projects/Gamma.md"}},
		{"typed aliases: isEmpty", `aliases.isEmpty()`, except("Areas/Garden.md", "Projects/Gamma.md")},
		{"typed tags: isEmpty", `tags.isEmpty()`, except("Areas/Garden.md", "Areas/Work.md", "Daily/2026-03-05.md", "Daily/2026-03-06.md", "Inbox.md",
			"Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Gamma.md", "Reading/Article One.md", "Reading/Books.md", "Unicode Ünï.md")},
		{"typed multitext: a text value", `labels.contains("solo")`, []string{"Inbox.md", "Projects/Gamma.md"}},
		{"typed multitext: containsAny", `labels.containsAny("duo", "solo")`, []string{"Inbox.md", "Projects/Gamma.md"}},
		{"typed multitext: containsAll", `labels.containsAll("solo", "duo")`, []string{"Inbox.md"}},
		{"typed multitext: a number item is numeric", `labels.contains(3)`, []string{"Projects/Beta.md"}},
		{"typed multitext: a number item, as a real", `labels.contains(3.0)`, []string{"Projects/Beta.md"}},
		{"typed multitext: a number is not a text item", `labels.contains("3")`, nil},
		{"typed multitext: a scalar number is not an item", `labels.contains(7)`, nil},
		{"typed multitext: a boolean is not an item", `labels.containsAny("true", 1)`, nil},
		{"typed multitext: an empty text is no item", `labels.contains("")`, nil},
		{"typed multitext: an empty text is empty", `labels.isEmpty()`, except("Inbox.md", "Misc/Spaced Out.md", "Projects/Beta.md", "Projects/Gamma.md", "Unicode Ünï.md")},
		{"typed multitext: a number and a boolean are not empty", `!labels.isEmpty() && !labels.contains("solo")`, []string{"Misc/Spaced Out.md", "Projects/Beta.md", "Unicode Ünï.md"}},
		{"untyped number item", `rating.contains(3)`, nil},
		{"isEmpty on a missing value", `rating.isEmpty()`, except("Areas/Garden.md", "Areas/Work.md", "Inbox.md", "Misc/Many.md", "Misc/Spaced Out.md", "Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Gamma.md", "Reading/Article One.md", "Reading/Books.md")},
		{"isEmpty on an empty list", `items.isEmpty() && file.hasProperty("items")`, []string{"Areas/Garden.md"}},
		{"isEmpty on null", `notes.isEmpty() && file.hasProperty("notes")`, []string{"Areas/Garden.md"}},
		{"isEmpty on an empty string", `note["note"].isEmpty() && file.hasProperty("note")`, []string{"Projects/Gamma.md"}},
		{"not isEmpty", `!kind.isEmpty()`, []string{"Areas/Garden.md", "Areas/Work.md", "Reading/Article One.md", "Reading/Books.md"}},

		// Operators and nesting.
		{"||", `status == "active" || rating > 4`, []string{"Areas/Garden.md", "Inbox.md", "Misc/Many.md", "Projects/Alpha.md", "Reading/Article One.md"}},
		{"&&", `status == "active" && rating > 4`, []string{"Projects/Alpha.md"}},
		{"|| and && precedence", `status == "done" || status == "active" && rating > 4`, []string{"Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md"}},
		{"parentheses", `(status == "done" || status == "active") && rating > 4`, []string{"Projects/Alpha.md"}},
		{"! of ||", `!(status == "active" || rating > 4)`, except("Areas/Garden.md", "Inbox.md", "Misc/Many.md", "Projects/Alpha.md", "Reading/Article One.md")},
		{"! on a missing property", `!(due > today())`, except("Projects/Gamma.md")},
		{"! on a value that does not convert", `!(rating > 3)`, except("Areas/Garden.md", "Misc/Many.md", "Projects/Alpha.md", "Reading/Article One.md")},
		{"!! is the statement", `!!(rating > 3)`, []string{"Areas/Garden.md", "Misc/Many.md", "Projects/Alpha.md", "Reading/Article One.md"}},
		{"! on an attachment row", `!(note.status == "active")`, except("Inbox.md", "Projects/Alpha.md")},
		{"|| keeps the missing", `rating > 3 || !(rating > 3)`, allFiles},
		{"spaces and newlines", "  status\n == \"active\"  ", []string{"Inbox.md", "Projects/Alpha.md"}},
		{"a quote in a string", `status == "say \"hi\""`, nil},
		{"escapes in a string", `status == 'a\tb\n\\'`, nil},
		{"a key with a space", `note["my key"] == 1`, nil},
		{"a unicode key", `note["日本"] == 1`, nil},
		{"a unicode bare key", `日本 == 1`, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := e.run(filterBase(tc.stmt))
			if err != nil {
				t.Fatalf("%s: %v", tc.stmt, err)
			}
			if got := paths(res); !slices.Equal(got, nonNil(tc.want)) {
				t.Errorf("%s\n got %q\nwant %q", tc.stmt, got, tc.want)
			}
			if int(res.Total) != len(tc.want) {
				t.Errorf("%s: total = %d, want %d", tc.stmt, res.Total, len(tc.want))
			}
			// Every statement is 0 or 1 and never NULL: not: [it] holds the other rows.
			other, err := e.run("filters:\n  not:\n    - " + quote(tc.stmt) + "\n")
			if err != nil {
				t.Fatal(err)
			}
			if got, want := paths(other), except(tc.want...); !slices.Equal(got, nonNil(want)) {
				t.Errorf("not: [%s]\n got %q\nwant %q", tc.stmt, got, want)
			}
		})
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
