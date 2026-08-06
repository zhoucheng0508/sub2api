package deterministicrisk

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	lexicalNatural    = "natural_language"
	lexicalQuoted     = "quoted_text"
	lexicalFilePath   = "file_path"
	lexicalIdentifier = "identifier"
	lexicalCode       = "code"
	lexicalLogField   = "log_field"
)

type lexicalRange struct {
	start    int
	end      int
	kind     string
	priority int
}

type lexicalDocument struct {
	text     string
	natural  string
	ranges   []lexicalRange
	clauseAt []int
	types    []string
}

var (
	fencedCodePattern = regexp.MustCompile("(?s)```.*?(?:```|$)")
	inlineCodePattern = regexp.MustCompile("`[^`\\r\\n]*`")
	quotedPatterns    = []*regexp.Regexp{
		regexp.MustCompile(`“[^”\r\n]*”`),
		regexp.MustCompile(`「[^」\r\n]*」`),
		regexp.MustCompile(`『[^』\r\n]*』`),
		regexp.MustCompile(`"[^"\r\n]{1,1000}"`),
		regexp.MustCompile(`(?m)(?:^|[\t (\[{=:,;])'[^'\r\n]{2,1000}'`),
	}
	filePathPattern        = regexp.MustCompile(`(?i)(?:[a-z]:[\\/]|\.{0,2}[\\/]|[a-z0-9_.-]+[\\/])[a-z0-9_.\\/\-]+`)
	fileNamePattern        = regexp.MustCompile(`(?i)\b[a-z][a-z0-9_.-]*\.(?:go|py|js|jsx|ts|tsx|vue|json|ya?ml|toml|ini|sql|html|css|log|txt|md|sh|ps1|java|rs|rb|php|c|cc|cpp|h|hpp|cs|kt|swift|scala|xml|env|cfg|conf)\b`)
	identifierPattern      = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9]*(?:_[A-Za-z0-9]+)+\b`)
	codeLinePrefixPattern  = regexp.MustCompile(`(?i)^(?:package|import|func|type|struct|class|def|const|let|var)\b`)
	controlCodeLinePattern = regexp.MustCompile(`(?i)^(?:return\b.*(?:;|\)|\}|\]|\+|-|\*|/)|(?:if|for|while|switch)\s*\(|(?:else|case)\b.*[:{])`)
	sqlCodeLinePattern     = regexp.MustCompile(`(?i)^(?:select\b.*\bfrom\b|insert\s+into\b|update\b.*\bset\b|delete\s+from\b|(?:create|alter)\s+table\b)`)
	logLinePattern         = regexp.MustCompile(`(?i)^(?:(?:trace|debug|info|warn|warning|error|fatal)\b|script completed\b|exit code\b|total output lines\b|at\s+\S+\s*\(|[a-z_][a-z0-9_.-]{1,64}\s*[:=]\s*\S+)`)
)

func buildLexicalDocument(text string) lexicalDocument {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	ranges := make([]lexicalRange, 0, 16)
	appendPatternRanges := func(pattern *regexp.Regexp, kind string, priority int) {
		for _, indexes := range pattern.FindAllStringIndex(text, -1) {
			ranges = append(ranges, lexicalRange{start: indexes[0], end: indexes[1], kind: kind, priority: priority})
		}
	}

	appendPatternRanges(fencedCodePattern, lexicalCode, 100)
	appendPatternRanges(inlineCodePattern, lexicalCode, 95)
	for _, pattern := range quotedPatterns {
		appendPatternRanges(pattern, lexicalQuoted, 90)
	}
	appendCodeAndLogLineRanges(text, &ranges)
	appendPatternRanges(filePathPattern, lexicalFilePath, 70)
	appendPatternRanges(fileNamePattern, lexicalFilePath, 70)
	appendPatternRanges(identifierPattern, lexicalIdentifier, 60)

	sort.SliceStable(ranges, func(i, j int) bool {
		if ranges[i].start == ranges[j].start {
			if ranges[i].priority == ranges[j].priority {
				return ranges[i].end > ranges[j].end
			}
			return ranges[i].priority > ranges[j].priority
		}
		return ranges[i].start < ranges[j].start
	})

	masked := []byte(text)
	seenTypes := make(map[string]bool, 6)
	for _, span := range ranges {
		if span.start < 0 || span.end > len(masked) || span.start >= span.end {
			continue
		}
		seenTypes[span.kind] = true
		for i := span.start; i < span.end; i++ {
			if masked[i] != '\n' {
				masked[i] = ' '
			}
		}
	}
	natural := string(masked)
	if containsNaturalText(natural) {
		seenTypes[lexicalNatural] = true
	}
	types := make([]string, 0, len(seenTypes))
	for _, kind := range []string{lexicalNatural, lexicalQuoted, lexicalFilePath, lexicalIdentifier, lexicalCode, lexicalLogField} {
		if seenTypes[kind] {
			types = append(types, kind)
		}
	}

	return lexicalDocument{
		text:     text,
		natural:  natural,
		ranges:   ranges,
		clauseAt: buildClauseIndex(natural),
		types:    types,
	}
}

func appendCodeAndLogLineRanges(text string, ranges *[]lexicalRange) {
	for start := 0; start < len(text); {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += start
		}
		line := strings.TrimSpace(text[start:end])
		kind := ""
		priority := 0
		if strings.HasPrefix(line, ">") {
			kind, priority = lexicalQuoted, 88
		} else if isCodeLikeLine(line) {
			kind, priority = lexicalCode, 85
		} else if logLinePattern.MatchString(line) {
			kind, priority = lexicalLogField, 80
		}
		if kind != "" {
			*ranges = append(*ranges, lexicalRange{start: start, end: end, kind: kind, priority: priority})
		}
		if end == len(text) {
			break
		}
		start = end + 1
	}
}

func isCodeLikeLine(line string) bool {
	if line == "" {
		return false
	}
	if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") ||
		strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "# ") {
		return true
	}
	if codeLinePrefixPattern.MatchString(line) || controlCodeLinePattern.MatchString(line) || sqlCodeLinePattern.MatchString(line) {
		return true
	}
	if (strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}")) ||
		(strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]")) {
		return strings.Contains(line, ":") || strings.Contains(line, ",")
	}
	if strings.HasPrefix(line, ">>> ") || strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "PS ") {
		return true
	}
	return strings.Contains(line, ":=") ||
		(strings.Contains(line, "{") && strings.Contains(line, "}") && strings.Contains(line, ";"))
}

func containsNaturalText(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

func buildClauseIndex(text string) []int {
	indexes := make([]int, len(text)+1)
	clause := 0
	inSeparatorRun := false
	for position, r := range text {
		size := utf8.RuneLen(r)
		if size < 1 {
			size = 1
		}
		for i := position; i < position+size && i < len(indexes); i++ {
			indexes[i] = clause
		}
		separator := isClauseSeparator(r)
		if separator {
			if !inSeparatorRun {
				clause++
			}
			inSeparatorRun = true
		} else if !unicode.IsSpace(r) {
			inSeparatorRun = false
		}
	}
	indexes[len(text)] = clause
	return indexes
}

func isClauseSeparator(r rune) bool {
	switch r {
	case ',', '，', ';', '；', '.', '。', '!', '！', '?', '？', '\n':
		return true
	default:
		return false
	}
}

func (document lexicalDocument) clause(position int) int {
	if position < 0 {
		return 0
	}
	if position >= len(document.clauseAt) {
		return document.clauseAt[len(document.clauseAt)-1]
	}
	return document.clauseAt[position]
}

func (document lexicalDocument) lexicalType(start, end int) string {
	bestPriority := -1
	kind := lexicalNatural
	for _, span := range document.ranges {
		if start >= span.end || end <= span.start {
			continue
		}
		if span.priority > bestPriority {
			bestPriority = span.priority
			kind = span.kind
		}
	}
	return kind
}
