package service

import (
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/Kcchouette/cardigann-go/definition"
	"gopkg.in/yaml.v3"

	"github.com/Kcchouette/gowlarr/internal/netutil"
	"github.com/Kcchouette/gowlarr/internal/store"
)

// rotationMu guards concurrent rewrites of the same local definition file.
var rotationMu sync.Map // defID -> *sync.Mutex

var defIDSlugRe = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// hostCharsetRe allowlists the characters a rotated host may contain:
// url.Parse accepts quotes inside a host, which would corrupt the YAML.
var hostCharsetRe = regexp.MustCompile(`^[a-zA-Z0-9.\-\[\]]+$`)

// maybePersistRotation rewrites links[0] in the matching definitions-local
// YAML and upserts the DB cache when the observed effective origin differs
// from the configured one. It never returns an error: a failed rotation
// persist must not fail the search that discovered it.
func maybePersistRotation(st *store.Store, defID, configuredBase, effectiveBase string) {
	if defID == "" || configuredBase == "" || effectiveBase == "" {
		return
	}
	cfgOrigin, ok := canonicalOrigin(configuredBase)
	if !ok {
		return
	}
	effOrigin, ok := canonicalOrigin(effectiveBase)
	if !ok || effOrigin == cfgOrigin {
		return
	}

	effURL, err := url.Parse(effectiveBase)
	if err != nil || (effURL.Scheme != "http" && effURL.Scheme != "https") || effURL.Host == "" {
		slog.Warn("domain rotation persist skipped: invalid effective base", "indexer", defID, "effective", effectiveBase)
		return
	}
	if !hostCharsetRe.MatchString(effURL.Host) {
		slog.Warn("domain rotation persist skipped: host charset not allowed", "indexer", defID, "host", effURL.Host)
		return
	}
	if !defIDSlugRe.MatchString(defID) || filepath.Base(defID) != defID {
		slog.Warn("domain rotation persist skipped: def id not a plain slug", "indexer", defID)
		return
	}
	if err := netutil.ValidateURL(effectiveBase); err != nil {
		slog.Warn("domain rotation persist skipped: validation failed", "indexer", defID, "effective", effectiveBase, "err", err)
		return
	}

	muIface, _ := rotationMu.LoadOrStore(defID, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	path, raw, ok := findLocalDefinition(defID)
	if !ok {
		// Upstream/synced definition: runtime rewrite only.
		return
	}
	newRaw, ok := rewriteLinksFirstEntry(raw, effOrigin)
	if !ok {
		return
	}
	if newRaw == raw {
		return
	}
	if err := writeFileAtomic(path, newRaw); err != nil {
		slog.Warn("domain rotation persist skipped: write failed", "indexer", defID, "path", path, "err", err)
		return
	}
	if err := st.SaveDefinition(defID, "local", "", newRaw); err != nil {
		slog.Warn("domain rotation: DB cache update failed", "indexer", defID, "err", err)
		return
	}
	slog.Info("domain rotation persisted", "indexer", defID, "from", cfgOrigin, "to", effOrigin, "path", path)
}

// canonicalOrigin returns scheme://host[:port] lowercased with
// scheme-default ports stripped, or ok=false when raw is unparseable.
func canonicalOrigin(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	switch {
	case scheme == "http" && strings.HasSuffix(host, ":80"):
		host = strings.TrimSuffix(host, ":80")
	case scheme == "https" && strings.HasSuffix(host, ":443"):
		host = strings.TrimSuffix(host, ":443")
	}
	return scheme + "://" + host, true
}

// findLocalDefinition scans LocalDefsDir for a file whose parsed definition
// ID matches defID (the filename is not guaranteed to equal the id).
func findLocalDefinition(defID string) (string, string, bool) {
	dir := LocalDefsDir()
	if dir == "" {
		return "", "", false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		def, err := definition.Parse(raw)
		if err != nil || def.ID != defID {
			continue
		}
		return path, string(raw), true
	}
	return "", "", false
}

// rewriteLinksFirstEntry locates the first scalar of the root-level links
// sequence via yaml.v3 (comments/formatting preserved: only that scalar's
// span is rewritten in the raw buffer) and replaces its origin with
// newOrigin. The previous base URL (the verbatim old token) is appended to
// the standard Cardigann legacylinks list (best-effort: a failed history
// insert logs a warning but the links[0] rewrite still applies). Notes of
// the former inline "rotated-from:" trail are stripped from the rewritten
// line.
//
// Guards (Warn + no write of links[0]): flow-style links collections (a
// trailing # comment would comment out the closing bracket), multi-line
// scalars, unexpected shapes.
func rewriteLinksFirstEntry(raw, newOrigin string) (string, bool) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		slog.Warn("domain rotation persist skipped: YAML parse failed", "err", err)
		return "", false
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		slog.Warn("domain rotation persist skipped: unexpected YAML root")
		return "", false
	}
	mapping := root.Content[0]
	var linksSeq, legacySeq *yaml.Node
	linksKeyIdx := -1
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		switch mapping.Content[i].Value {
		case "links":
			if mapping.Content[i+1].Kind == yaml.SequenceNode {
				linksSeq = mapping.Content[i+1]
				linksKeyIdx = i
			}
		case "legacylinks":
			legacySeq = mapping.Content[i+1]
		}
	}
	if linksSeq == nil || linksSeq.Style == yaml.FlowStyle || len(linksSeq.Content) == 0 {
		slog.Warn("domain rotation persist skipped: links sequence missing or flow-style")
		return "", false
	}
	scalar := linksSeq.Content[0]
	if scalar.Kind != yaml.ScalarNode || scalar.Style == yaml.FlowStyle || strings.Contains(scalar.Value, "\n") {
		slog.Warn("domain rotation persist skipped: links[0] scalar shape unsupported")
		return "", false
	}

	// Rewrite only the scalar token in the raw buffer: start at (Line,
	// Column), end at the quote-aware end of the token (EOL or " #").
	lines := strings.Split(raw, "\n")
	if scalar.Line < 1 || scalar.Line > len(lines) {
		slog.Warn("domain rotation persist skipped: scalar line out of range")
		return "", false
	}
	line := lines[scalar.Line-1]
	col := scalar.Column - 1
	if col < 0 || col >= len(line) {
		slog.Warn("domain rotation persist skipped: scalar column out of range")
		return "", false
	}
	tokenEnd := scalarTokenEnd(line, col)
	if tokenEnd <= col {
		slog.Warn("domain rotation persist skipped: could not bound scalar token")
		return "", false
	}

	oldToken := line[col:tokenEnd]
	oldURL := strings.Trim(oldToken, `"'`)
	// Keep only the path: a query on links[0] would not survive an origin
	// swap cleanly (Cardigann bases are bare origins in practice).
	pathSuffix := "/"
	if u, err := url.Parse(oldURL); err == nil && u.Path != "" {
		pathSuffix = u.Path
	}
	// Preserve quoting style of the original token.
	var newToken string
	switch {
	case strings.HasPrefix(oldToken, `"`):
		newToken = `"` + newOrigin + pathSuffix + `"`
	case strings.HasPrefix(oldToken, `'`):
		newToken = `'` + newOrigin + pathSuffix + `'`
	default:
		newToken = newOrigin + pathSuffix
	}

	rest := stripRotatedFromNote(line[tokenEnd:])
	lines[scalar.Line-1] = line[:col] + newToken + rest

	// History insert (best-effort): the links sequence rewrite above did
	// not change the line count, so parse Line numbers stay valid.
	var linksNextKeyLine int
	if linksKeyIdx >= 0 && linksKeyIdx+2 < len(mapping.Content) {
		linksNextKeyLine = mapping.Content[linksKeyIdx+2].Line
	}
	lines, _ = appendLegacyLink(lines, legacySeq, linksNextKeyLine, linksSeq, oldURL)
	return strings.Join(lines, "\n"), true
}

// stripRotatedFromNote removes a legacy inline "rotated-from: <url>" note
// from the remainder of a rewritten links[0] line, preserving any user
// comment text around it. The delivered trail used a single "#" comment
// marker, optionally prefixed with user text joined by " | ".
func stripRotatedFromNote(rest string) string {
	idx := strings.Index(rest, "rotated-from:")
	if idx < 0 {
		return rest
	}
	before := strings.TrimRight(rest[:idx], " \t")
	// Drop the " | " (or "|") joiner that preceded the note, if any.
	before = strings.TrimSuffix(before, "|")
	before = strings.TrimRight(before, " \t")
	// A remainder that is only a comment marker (possibly with spaces)
	// was entirely the note: drop it.
	if strings.TrimLeft(before, " \t") == "#" || strings.TrimSpace(before) == "" {
		return ""
	}
	return before
}

// appendLegacyLink inserts oldURL into the legacylinks sequence (created
// after the links block when absent). Best-effort: exotic shapes log a
// warning and return ok=false; the caller keeps the links[0] rewrite.
// Note: comments sitting after the last existing item but before the next
// root key will end up after the new item — acceptable for this trail.
func appendLegacyLink(lines []string, legacySeq *yaml.Node, linksNextKeyLine int, linksSeq *yaml.Node, oldURL string) ([]string, bool) {
	if legacySeq != nil {
		if legacySeq.Kind != yaml.SequenceNode {
			slog.Warn("domain rotation history skipped: legacylinks is not a sequence")
			return lines, false
		}
		for _, item := range legacySeq.Content {
			if strings.Contains(item.Value, "\n") {
				slog.Warn("domain rotation history skipped: multi-line legacylinks item")
				return lines, false
			}
			if strings.Trim(item.Value, `"'`) == oldURL {
				return lines, true // already recorded
			}
		}
		if legacySeq.Style == yaml.FlowStyle {
			return appendLegacyFlow(lines, legacySeq, oldURL)
		}
		if len(legacySeq.Content) == 0 {
			slog.Warn("domain rotation history skipped: empty block legacylinks")
			return lines, false
		}
		// Dash column comes from the SEQUENCE node, not the item (yaml.v3
		// items report the value column).
		indent := strings.Repeat(" ", legacySeq.Column-1)
		insertIdx := legacySeq.Content[len(legacySeq.Content)-1].Line // 1-based → insert after
		return slices.Insert(lines, insertIdx, indent+"- "+oldURL), true
	}

	// No legacylinks key: insert a new block after the links sequence
	// (before the next root key, or at EOF preserving the trailing newline).
	if linksSeq == nil || len(linksSeq.Content) == 0 {
		return lines, false
	}
	itemIndent := strings.Repeat(" ", linksSeq.Column-1)
	block := []string{"legacylinks:", itemIndent + "- " + oldURL}
	if linksNextKeyLine > 0 {
		return slices.Insert(lines, linksNextKeyLine-1, block...), true
	}
	// EOF: insert before the final empty element when raw ended with \n.
	insertIdx := len(lines)
	if insertIdx > 0 && lines[insertIdx-1] == "" {
		insertIdx--
	}
	return slices.Insert(lines, insertIdx, block...), true
}

// appendLegacyFlow appends oldURL to a single-line flow sequence. Items
// containing "]" or quotes (IPv6 literals, quoted strings) defeat a naive
// bracket search and are refused.
func appendLegacyFlow(lines []string, legacySeq *yaml.Node, oldURL string) ([]string, bool) {
	for _, item := range legacySeq.Content {
		if strings.ContainsAny(item.Value, `]"`) {
			slog.Warn("domain rotation history skipped: legacylinks flow item defeats bracket search")
			return lines, false
		}
	}
	row := legacySeq.Line - 1
	if row < 0 || row >= len(lines) {
		return lines, false
	}
	line := lines[row]
	closeIdx := strings.LastIndex(line, "]")
	if closeIdx < 0 {
		slog.Warn("domain rotation history skipped: legacylinks flow closing bracket not on key line")
		return lines, false
	}
	var inserted string
	if len(legacySeq.Content) == 0 {
		inserted = oldURL // [] → [url], no comma
	} else {
		inserted = ", " + oldURL
	}
	lines[row] = line[:closeIdx] + inserted + line[closeIdx:]
	return lines, true
}

// scalarTokenEnd returns the index (exclusive) in line where the scalar
// token starting at col ends: plain scalars end at whitespace before a
// comment or EOL; quoted scalars end after the matching closing quote.
func scalarTokenEnd(line string, col int) int {
	if col >= len(line) {
		return col
	}
	switch line[col] {
	case '"', '\'':
		q := line[col]
		for i := col + 1; i < len(line); i++ {
			if line[i] == '\\' && q == '"' {
				i++
				continue
			}
			if line[i] == q {
				return i + 1
			}
		}
		return len(line) // unterminated: treat to EOL
	default:
		end := len(line)
		for i := col; i < len(line); i++ {
			if line[i] == '#' && i > col && (line[i-1] == ' ' || line[i-1] == '\t') {
				end = i
				break
			}
		}
		// Plain scalars: drop trailing whitespace so it cannot leak into
		// the rewritten token / path suffix.
		for end > col && (line[end-1] == ' ' || line[end-1] == '\t') {
			end--
		}
		return end
	}
}

// writeFileAtomic writes data to path via a temp file in the same directory
// plus a rename.
func writeFileAtomic(path, data string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".rotation-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.WriteString(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// rotationAware is implemented by providers that can report an origin
// rotation observed during the last search. ID is satisfied by
// search.Provider (the cardigann adapter delegates it).
type rotationAware interface {
	ID() string
	EffectiveBaseURL() string
	ConfiguredBaseURL() string
}

// persistIfRotated persists a rotation discovered by p; never fails.
func persistIfRotated(st *store.Store, p rotationAware) {
	if st == nil || p == nil {
		return
	}
	maybePersistRotation(st, p.ID(), p.ConfiguredBaseURL(), p.EffectiveBaseURL())
}
