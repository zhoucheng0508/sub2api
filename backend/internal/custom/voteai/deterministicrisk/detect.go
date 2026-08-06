package deterministicrisk

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/custom/voteai/inputprovenance"
)

type signalPattern struct {
	label             string
	pattern           *regexp.Regexp
	explicitExecution bool
	continuation      bool
}

type tokenMatch struct {
	label             string
	value             string
	start             int
	end               int
	clause            int
	lexicalType       string
	explicitExecution bool
	continuation      bool
}

type documentEvidence struct {
	document           lexicalDocument
	intents            []tokenMatch
	targets            []tokenMatch
	unauthorized       []tokenMatch
	actions            []tokenMatch
	allIntents         []tokenMatch
	allTargets         []tokenMatch
	allUnauthorized    []tokenMatch
	allActions         []tokenMatch
	negationDetected   bool
	defensiveDetected  bool
	metaDetected       bool
	authorizedDetected bool
}

const (
	maxNaturalSignalMatches   = 128
	maxAuxiliarySignalMatches = 64
	maxAuxiliaryScanPerRule   = 512
	maxLinkedContextTurns     = 4
)

var (
	intentPatterns = []signalPattern{
		newSignalPattern("dangerous_intent_zh", `绕过|跳过|破解|爆破|盗取|窃取|偷取|劫持|提取|导出`, false, false),
		newSignalPattern("dangerous_intent_en", `(?i)\b(?:bypass(?:es|ed|ing)?|evad(?:e|es|ed|ing)|crack(?:s|ed|ing)?|brute[ -]?force|steal(?:s|ing)?|stole|stolen|exfiltrat(?:e|es|ed|ing|ion)|hijack(?:s|ed|ing)?|extract(?:s|ed|ing)?|dump(?:s|ed|ing)?|harvest(?:s|ed|ing)?)\b`, false, false),
		newSignalPattern("dangerous_intent_identifier", `(?i)(?:\b|[_./\\-])(?:bypass|evade|crack|steal|exfiltrate|hijack|extract)(?:$|[_./\\-])`, false, false),
	}
	targetPatterns = []signalPattern{
		newSignalPattern("credential_target_zh", `登录|认证|授权|访问控制|访问权限|所有权校验|密码|口令|凭据|密钥|令牌|验证码|私钥|会话|账号|账户`, false, false),
		newSignalPattern("credential_target_en", `(?i)\b(?:login|auth|authentication|authorization|access[ -]control|ownership[ -](?:check|verification)|passwords?|passcodes?|credentials?|api[ -]keys?|tokens?|cookies?|secrets?|private[ -]keys?|verification[ -]codes?|sessions?|accounts?|mfa|2fa)\b`, false, false),
		newSignalPattern("credential_target_identifier", `(?i)(?:\b|[_./\\-])(?:login|auth|authentication|authorization|password|credential|api[_-]?key|token|cookie|secret|session|account)(?:$|[_./\\-])`, false, false),
	}
	unauthorizedPatterns = []signalPattern{
		newSignalPattern("unauthorized_target_zh", `未经授权|未授权|没有授权|无需授权|不需要授权|无视授权|忽略授权|不验证所有权|跳过所有权|绕过所有权|不属于(?:我|本人|自己)|他人|别人|其他用户|陌生人`, false, false),
		newSignalPattern("unauthorized_target_en", `(?i)\b(?:unauthori[sz]ed|without[ -]authori[sz]ation|ignore[ -]authori[sz]ation|no[ -]authori[sz]ation|required[ -]no[ -]permission|someone[ -]else(?:'s)?|another[ -]user(?:'s)?|other[ -]users?|not[ -]mine|not[ -]my[ -]account|skip[ -]ownership[ -](?:check|verification))\b`, false, false),
		newSignalPattern("unauthorized_target_identifier", `(?i)(?:\b|[_./\\-])(?:unauthori[sz]ed|unowned|third[_-]?party)(?:$|[_./\\-])`, false, false),
	}
	actionPatterns = []signalPattern{
		newSignalPattern("direct_help_zh", `(?:帮我|教我|请|替我|直接)(?:[^，。！？；;\n]{0,24})?(?:绕过|破解|爆破|盗取|窃取|提取|导出|写|编写|生成|制作|执行|运行|自动化)`, true, false),
		newSignalPattern("deliver_artifact_zh", `(?:给我|提供|输出|写成|编写|生成|制作|执行|运行|自动化|批量)(?:[^，。！？；;\n]{0,16})?(?:脚本|代码|命令|步骤|方法|载荷|payload)`, true, false),
		newSignalPattern("inquiry_zh", `如何|怎么|怎样|具体步骤|详细步骤|操作步骤|教程|方法`, true, false),
		newSignalPattern("artifact_zh", `脚本|代码|命令|载荷|payload|自动化|批量`, false, false),
		newSignalPattern("continuation_zh", `继续|接着|按上面|照上面|根据上面|写成脚本|写成代码|再具体(?:一点)?|更详细(?:一点)?|执行上面`, true, true),
		newSignalPattern("direct_help_en", `(?i)\b(?:show|help|teach|give|provide|write|generate|build|execute|run|automate|return|output)\b(?:[ \t]+\S+){0,8}[ \t]+\b(?:bypass|evade|crack|steal|exfiltrate|extract|scripts?|code|commands?|payload|steps?)\b`, true, false),
		newSignalPattern("inquiry_en", `(?i)\b(?:how[ -]to|step[ -]by[ -]step|specific[ -]steps?|detailed[ -]steps?|instructions?|tutorial|method)\b`, true, false),
		newSignalPattern("artifact_en", `(?i)\b(?:scripts?|working[ -]code|source[ -]code|commands?|payloads?|automation)\b`, false, false),
		newSignalPattern("artifact_identifier", `(?i)(?:\b|[_./\\-])(?:script|code|command|payload|automation)(?:$|[_./\\-])`, false, false),
		newSignalPattern("continuation_en", `(?i)\b(?:continue|go[ -]on|carry[ -]on|turn[ -]it[ -]into[ -](?:a[ -])?(?:script|code)|make[ -]it[ -]more[ -]specific|more[ -]detail|execute[ -]the[ -]above)\b`, true, true),
	}
	negationPattern               = regexp.MustCompile(`(?i)(?:不要|别|禁止|不允许|不得|无需|无须|不用|停止|避免|切勿|不能|不可|不应该|\b(?:do[ -]not|don['’]?t|never|must[ -]not|should[ -]not|no[ -]need[ -]to|cannot|can['’]?t|is[ -]not[ -]allowed|are[ -]not[ -]allowed|prohibited)\b)`)
	defensivePattern              = regexp.MustCompile(`(?i)(?:防止|保护|预防|检测|审计|修复|加固|防御|风控|防刷|反钓鱼|安全校验|漏洞修复|合规|事件响应|测试|验证|\b(?:prevent|protect|detect|audit|fix|repair|mitigat(?:e|es|ed|ing|ion)|harden|defen[cs](?:e|ive)|risk[ -]control|anti[ -]phishing|incident[ -]response|test|verify|validate)\b)`)
	metaPattern                   = regexp.MustCompile(`(?i)(?:分析|解释|翻译|总结|评估|审阅|审核|误报|漏报|为什么危险|为何危险|为什么被拦截|规则|关键词|分类器|提示词|测试用例|日志|\b(?:analy[sz](?:e|es|ed|ing)|explain(?:s|ed|ing)?|translat(?:e|es|ed|ing|ion)|summari[sz](?:e|es|ed|ing|ation)|evaluat(?:e|es|ed|ing|ion)|review(?:s|ed|ing)?|false[ -](?:positive|negative)|rules?|keywords?|classifiers?|prompts?|test[ -]cases?|logs?)\b|\bwhy\b[^.!?\n]{0,80}\bdangerous\b)`)
	authorizedPattern             = regexp.MustCompile(`(?i)(?:我自己的|我自有|本人所有|自有系统|明确授权|经授权|合法授权|授权范围|测试环境|隔离环境|沙箱|\b(?:my[ -]own|owned[ -]by[ -]me|explicitly[ -]authori[sz]ed|with[ -]permission|authori[sz]ed[ -](?:test|system|environment)|sandbox|ctf|lab)\b)`)
	contrastPattern               = regexp.MustCompile(`(?i)(?:但是|但要|而是|却要|反而|直接|仍然|还是|然后|\b(?:but|however|instead|rather|directly|just|then|still)\b)`)
	negatedScopeObjectPattern     = regexp.MustCompile(`(?i)(?:提醒|警告|解释|讨论|提及|说明|披露|告诉|\b(?:warn|mention|explain|discuss|tell)\b)`)
	postposedNegationPattern      = regexp.MustCompile(`(?i)^(?:[^，。！？；;.!?\n]{0,120})(?:不允许|禁止|不可|不能|不应该|违法|违规|\b(?:is|are|would[ -]be)?[ -]*(?:not[ -]allowed|prohibited)|\b(?:cannot|can['’]?t|must[ -]not|should[ -]not)\b)`)
	politeImperativePrefixPattern = regexp.MustCompile(`(?i)^(?:(?:请|麻烦|现在|马上|直接|please|kindly)\s*)*$`)
)

func newSignalPattern(label, expression string, explicitExecution, continuation bool) signalPattern {
	return signalPattern{
		label:             label,
		pattern:           regexp.MustCompile(expression),
		explicitExecution: explicitExecution,
		continuation:      continuation,
	}
}

// Detect evaluates exactly one normalized audit target. It never scans a raw
// request envelope and uses only explicitly linked supporting context.
func Detect(input Input) Result {
	if !targetEligible(input.Target) {
		return None()
	}

	targetEvidence := analyzeDocument(input.Target.Text)
	if strings.TrimSpace(targetEvidence.document.text) == "" {
		return None()
	}
	metadataExcluded := collectMetadataExcluded(input)
	confirmedSource := targetSourceConsistent(input.Target)

	if combination, ok := completeNearbyCombination(targetEvidence); ok {
		explicitExecution := hasExplicitHarmfulExecution(targetEvidence, combination)
		if targetEvidence.authorizedDetected && (!explicitExecution || !hasExplicitThirdParty(combination)) {
			return makeResult(LevelCandidate, input.Target, targetEvidence, combination, metadataExcluded, "")
		}
		if !explicitExecution {
			if riskSignalsScopedByContextGate(targetEvidence) {
				return None()
			}
			return makeResult(LevelCandidate, input.Target, targetEvidence, combination, metadataExcluded, "")
		}
		if confirmedSource {
			return makeResult(LevelConfirmed, input.Target, targetEvidence, combination, metadataExcluded, "")
		}
		return makeResult(LevelCandidate, input.Target, targetEvidence, combination, metadataExcluded, "")
	}

	if result, ok := detectLinkedContinuation(input, targetEvidence, metadataExcluded, confirmedSource); ok {
		return result
	}

	if candidateCombination(targetEvidence) {
		if targetEvidence.authorizedDetected {
			matches := candidateMatches(targetEvidence)
			return makeResult(LevelCandidate, input.Target, targetEvidence, matches, metadataExcluded, "")
		}
		if riskSignalsScopedByContextGate(targetEvidence) || effectiveNegationOnly(targetEvidence) {
			return None()
		}
		matches := candidateMatches(targetEvidence)
		return makeResult(LevelCandidate, input.Target, targetEvidence, matches, metadataExcluded, "")
	}
	return None()
}

func targetEligible(target AuditTarget) bool {
	if target.Kind == inputprovenance.TargetNoNewUserIntent || target.Source == inputprovenance.SourceTrustedMetadata {
		return false
	}
	if target.MetadataKind != "" && target.MetadataKind != inputprovenance.MetadataNone {
		return false
	}
	switch target.Kind {
	case inputprovenance.TargetUserRequest, inputprovenance.TargetClientInstruction:
		return true
	case inputprovenance.TargetToolContinuation:
		return target.LinkedToUserIntent
	default:
		return false
	}
}

func targetSourceConsistent(target AuditTarget) bool {
	switch target.Kind {
	case inputprovenance.TargetUserRequest:
		return target.Source == inputprovenance.SourceEndUser
	case inputprovenance.TargetClientInstruction:
		return target.Source == inputprovenance.SourceClientInstruction
	case inputprovenance.TargetToolContinuation:
		return target.LinkedToUserIntent && target.Source == inputprovenance.SourceToolOutput
	default:
		return false
	}
}

func analyzeDocument(text string) documentEvidence {
	document := buildLexicalDocument(text)
	allIntents := findMatches(document, intentPatterns)
	allTargets := findMatches(document, targetPatterns)
	allUnauthorized := findMatches(document, unauthorizedPatterns)
	allActions := findMatches(document, actionPatterns)

	intents := unnegatedNaturalMatches(document, allIntents)
	actions := unnegatedNaturalMatches(document, allActions)
	actions = appendImperativeActions(document, intents, actions)
	return documentEvidence{
		document:           document,
		intents:            intents,
		targets:            naturalMatches(allTargets),
		unauthorized:       naturalMatches(allUnauthorized),
		actions:            actions,
		allIntents:         allIntents,
		allTargets:         allTargets,
		allUnauthorized:    allUnauthorized,
		allActions:         allActions,
		negationDetected:   negationPattern.MatchString(document.natural),
		defensiveDetected:  defensivePattern.MatchString(document.natural),
		metaDetected:       metaPattern.MatchString(document.natural),
		authorizedDetected: authorizedPattern.MatchString(document.natural),
	}
}

func findMatches(document lexicalDocument, patterns []signalPattern) []tokenMatch {
	natural := make([]tokenMatch, 0, 12)
	auxiliary := make([]tokenMatch, 0, 8)
	for _, candidate := range patterns {
		if remaining := maxNaturalSignalMatches - len(natural); remaining > 0 {
			for _, indexes := range candidate.pattern.FindAllStringIndex(document.natural, remaining) {
				match, ok := newTokenMatch(document, candidate, indexes)
				if !ok || match.lexicalType != lexicalNatural {
					continue
				}
				natural = append(natural, match)
			}
		}
		if len(auxiliary) >= maxAuxiliarySignalMatches {
			continue
		}
		for _, indexes := range candidate.pattern.FindAllStringIndex(document.text, maxAuxiliaryScanPerRule) {
			match, ok := newTokenMatch(document, candidate, indexes)
			if !ok || match.lexicalType == lexicalNatural {
				continue
			}
			auxiliary = append(auxiliary, match)
			if len(auxiliary) >= maxAuxiliarySignalMatches {
				break
			}
		}
	}
	matches := append(natural, auxiliary...)
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].start == matches[j].start {
			return matches[i].end < matches[j].end
		}
		return matches[i].start < matches[j].start
	})
	return deduplicateTokenMatches(matches)
}

func newTokenMatch(document lexicalDocument, candidate signalPattern, indexes []int) (tokenMatch, bool) {
	if len(indexes) != 2 || indexes[0] < 0 || indexes[1] > len(document.text) || indexes[0] >= indexes[1] {
		return tokenMatch{}, false
	}
	value := strings.TrimSpace(document.text[indexes[0]:indexes[1]])
	if value == "" {
		return tokenMatch{}, false
	}
	return tokenMatch{
		label:             candidate.label,
		value:             truncateRunes(value, 64),
		start:             indexes[0],
		end:               indexes[1],
		clause:            document.clause(indexes[0]),
		lexicalType:       document.lexicalType(indexes[0], indexes[1]),
		explicitExecution: candidate.explicitExecution,
		continuation:      candidate.continuation,
	}, true
}

func naturalMatches(matches []tokenMatch) []tokenMatch {
	result := make([]tokenMatch, 0, len(matches))
	for _, match := range matches {
		if match.lexicalType == lexicalNatural {
			result = append(result, match)
		}
	}
	return result
}

func unnegatedNaturalMatches(document lexicalDocument, matches []tokenMatch) []tokenMatch {
	result := make([]tokenMatch, 0, len(matches))
	for _, match := range matches {
		if match.lexicalType != lexicalNatural || matchIsNegated(document, match) {
			continue
		}
		result = append(result, match)
	}
	return result
}

func appendImperativeActions(document lexicalDocument, intents, actions []tokenMatch) []tokenMatch {
	for _, intent := range intents {
		clauseStart := previousClauseBoundary(document.natural, intent.start)
		prefix := strings.TrimSpace(document.natural[clauseStart:intent.start])
		if !politeImperativePrefixPattern.MatchString(prefix) {
			continue
		}
		actions = append(actions, tokenMatch{
			label:             "imperative",
			value:             intent.value,
			start:             intent.start,
			end:               intent.end,
			clause:            intent.clause,
			lexicalType:       lexicalNatural,
			explicitExecution: true,
		})
	}
	return deduplicateTokenMatches(actions)
}

func matchIsNegated(document lexicalDocument, match tokenMatch) bool {
	start := previousClauseBoundary(document.natural, match.start)
	prefix := document.natural[start:match.start]
	locations := negationPattern.FindAllStringIndex(prefix, -1)
	if len(locations) == 0 {
		end := nextClauseBoundary(document.natural, match.end)
		return postposedNegationPattern.MatchString(document.natural[match.end:end])
	}
	last := locations[len(locations)-1]
	afterNegation := prefix[last[1]:]
	if contrastPattern.MatchString(afterNegation) {
		return false
	}
	if negatedScopeObjectPattern.MatchString(afterNegation) && strings.TrimSpace(afterNegation) != "" {
		return false
	}
	return len([]rune(afterNegation)) <= 24
}

func previousClauseBoundary(text string, position int) int {
	if position <= 0 {
		return 0
	}
	if position > len(text) {
		position = len(text)
	}
	if index := strings.LastIndexAny(text[:position], ",，;；.。!！?？\n"); index >= 0 {
		_, size := runeAt(text, index)
		return index + size
	}
	return 0
}

func runeAt(text string, position int) (rune, int) {
	for position > 0 && (text[position]&0xc0) == 0x80 {
		position--
	}
	r := rune(text[position])
	size := 1
	if r >= 0x80 {
		runes := []rune(text[position:])
		if len(runes) > 0 {
			r = runes[0]
			size = len(string(r))
		}
	}
	return r, size
}

func completeNearbyCombination(evidence documentEvidence) ([]tokenMatch, bool) {
	best := nearbyCombination(evidence.intents, evidence.targets, evidence.unauthorized, evidence.actions)
	return deduplicateTokenMatches(best), len(best) > 0
}

func detectLinkedContinuation(input Input, target documentEvidence, metadataExcluded []string, confirmedSource bool) (Result, bool) {
	continuations := make([]tokenMatch, 0, len(target.actions))
	for _, action := range target.actions {
		if action.continuation {
			continuations = append(continuations, action)
		}
	}
	if len(continuations) == 0 || target.metaDetected || target.defensiveDetected || target.authorizedDetected {
		return Result{}, false
	}
	expectedRole := roleForSource(input.Target.Source)
	supports := make([]documentEvidence, 0, maxLinkedContextTurns)
	for index := len(input.SupportingContext) - 1; index >= 0; index-- {
		context := input.SupportingContext[index]
		if !context.DirectlyLinked || context.Purpose != inputprovenance.PurposeSupportingContext ||
			context.Source != input.Target.Source || context.Role != expectedRole ||
			context.Source == inputprovenance.SourceTrustedMetadata ||
			(context.MetadataKind != "" && context.MetadataKind != inputprovenance.MetadataNone) {
			continue
		}
		support := analyzeDocument(context.Text)
		if support.metaDetected || support.defensiveDetected || support.authorizedDetected || effectiveNegationOnly(support) {
			continue
		}
		if len(support.intents)+len(support.targets)+len(support.unauthorized) == 0 {
			continue
		}
		supports = append(supports, support)
		if len(supports) == maxLinkedContextTurns {
			break
		}
	}
	if len(supports) == 0 {
		return Result{}, false
	}
	merged := target
	for _, support := range supports {
		merged = mergeDiagnosticsEvidence(merged, support)
	}
	contextMatches, complete := linkedContextGoal(supports)
	matches := append(contextMatches, continuations[0])
	excerpt := linkedContextExcerpt(supports, target, continuations)
	if !complete {
		if linkedContextHasCandidateGoal(supports) {
			return makeResult(LevelCandidate, input.Target, merged, matches, metadataExcluded, excerpt), true
		}
		return Result{}, false
	}
	if confirmedSource {
		return makeResult(LevelConfirmed, input.Target, merged, matches, metadataExcluded, excerpt), true
	}
	return makeResult(LevelCandidate, input.Target, merged, matches, metadataExcluded, excerpt), true
}

func linkedContextGoal(supports []documentEvidence) ([]tokenMatch, bool) {
	var intents, targets, unauthorized []tokenMatch
	for _, support := range supports {
		intents = append(intents, support.intents...)
		targets = append(targets, support.targets...)
		unauthorized = append(unauthorized, support.unauthorized...)
	}
	matches := make([]tokenMatch, 0, 3)
	if len(intents) > 0 {
		matches = append(matches, intents[0])
	}
	if len(targets) > 0 {
		matches = append(matches, targets[0])
	}
	if len(unauthorized) > 0 {
		matches = append(matches, unauthorized[0])
	}
	complete := len(intents) > 0 && len(targets) > 0 && len(unauthorized) > 0
	return deduplicateTokenMatches(matches), complete
}

func linkedContextHasCandidateGoal(supports []documentEvidence) bool {
	hasIntent, hasTarget := false, false
	for _, support := range supports {
		hasIntent = hasIntent || len(support.intents) > 0
		hasTarget = hasTarget || len(support.targets) > 0
	}
	return hasIntent && hasTarget
}

func nearbyCombination(groups ...[]tokenMatch) []tokenMatch {
	if len(groups) == 0 {
		return nil
	}
	windows := make(map[int]struct{})
	for _, group := range groups {
		if len(group) == 0 {
			return nil
		}
		for _, match := range group {
			windows[match.clause] = struct{}{}
			if match.clause > 0 {
				windows[match.clause-1] = struct{}{}
			}
		}
	}
	starts := make([]int, 0, len(windows))
	for start := range windows {
		starts = append(starts, start)
	}
	sort.Ints(starts)
	bestWidth := int(^uint(0) >> 1)
	var best []tokenMatch
	for _, start := range starts {
		combination := make([]tokenMatch, 0, len(groups))
		complete := true
		for _, group := range groups {
			selected, ok := firstMatchInClauseWindow(group, start, start+1)
			if !ok {
				complete = false
				break
			}
			combination = append(combination, selected)
		}
		if !complete {
			continue
		}
		if width := spanWidth(combination); width < bestWidth {
			bestWidth, best = width, combination
		}
	}
	return best
}

func firstMatchInClauseWindow(matches []tokenMatch, minimum, maximum int) (tokenMatch, bool) {
	for _, match := range matches {
		if match.clause >= minimum && match.clause <= maximum {
			return match, true
		}
	}
	return tokenMatch{}, false
}

func roleForSource(source inputprovenance.Source) inputprovenance.Role {
	switch source {
	case inputprovenance.SourceEndUser:
		return inputprovenance.RoleUser
	case inputprovenance.SourceToolOutput:
		return inputprovenance.RoleTool
	case inputprovenance.SourceClientInstruction:
		return inputprovenance.RoleDeveloper
	case inputprovenance.SourceAssistantResponse:
		return inputprovenance.RoleAssistant
	default:
		return ""
	}
}

func candidateCombination(evidence documentEvidence) bool {
	naturalIntent := len(evidence.intents) > 0
	naturalTarget := len(evidence.targets) > 0
	naturalUnauthorized := len(evidence.unauthorized) > 0
	naturalAction := len(evidence.actions) > 0
	if naturalIntent && naturalTarget && (naturalUnauthorized || naturalAction) {
		return true
	}
	if naturalAction && len(evidence.allIntents) > 0 && len(evidence.allTargets) > 0 && len(evidence.allUnauthorized) > 0 {
		return true
	}
	return false
}

func candidateMatches(evidence documentEvidence) []tokenMatch {
	matches := preferredMatches(evidence.intents, evidence.allIntents)
	matches = append(matches, preferredMatches(evidence.targets, evidence.allTargets)...)
	matches = append(matches, preferredMatches(evidence.unauthorized, evidence.allUnauthorized)...)
	matches = append(matches, preferredMatches(evidence.actions, evidence.allActions)...)
	return deduplicateTokenMatches(matches)
}

func preferredMatches(natural, all []tokenMatch) []tokenMatch {
	if len(natural) > 0 {
		return append([]tokenMatch(nil), natural...)
	}
	return append([]tokenMatch(nil), all...)
}

func hasExplicitHarmfulExecution(evidence documentEvidence, combination []tokenMatch) bool {
	var intents []tokenMatch
	for _, match := range combination {
		if containsToken(evidence.intents, match) {
			intents = append(intents, match)
		}
	}
	for _, action := range evidence.actions {
		if !action.explicitExecution {
			continue
		}
		nearIntent := false
		overlapsIntent := false
		for _, intent := range intents {
			if abs(action.clause-intent.clause) <= 1 {
				nearIntent = true
			}
			if action.start < intent.end && intent.start < action.end {
				overlapsIntent = true
			}
		}
		if !nearIntent {
			continue
		}
		actionContainsGate := defensivePattern.MatchString(action.value) || metaPattern.MatchString(action.value)
		if actionContainsGate {
			continue
		}
		if overlapsIntent || (!clauseContainsGate(evidence.document, action, defensivePattern) && !clauseContainsGate(evidence.document, action, metaPattern)) {
			return true
		}
	}
	return false
}

func hasExplicitThirdParty(matches []tokenMatch) bool {
	for _, match := range matches {
		value := strings.ToLower(match.value)
		if strings.Contains(value, "他人") || strings.Contains(value, "别人") ||
			strings.Contains(value, "其他用户") || strings.Contains(value, "陌生人") ||
			strings.Contains(value, "不属于") || strings.Contains(value, "someone else") ||
			strings.Contains(value, "another user") || strings.Contains(value, "other user") ||
			strings.Contains(value, "not mine") || strings.Contains(value, "not my account") {
			return true
		}
	}
	return false
}

func riskSignalsScopedByContextGate(evidence documentEvidence) bool {
	if !evidence.metaDetected && !evidence.defensiveDetected {
		return false
	}
	for _, match := range append(append([]tokenMatch(nil), evidence.intents...), evidence.actions...) {
		if !clauseContainsGate(evidence.document, match, metaPattern) &&
			!clauseContainsGate(evidence.document, match, defensivePattern) {
			return false
		}
	}
	return true
}

func clauseContainsGate(document lexicalDocument, match tokenMatch, pattern *regexp.Regexp) bool {
	start := previousClauseBoundary(document.natural, match.start)
	end := nextClauseBoundary(document.natural, match.end)
	return pattern.MatchString(document.natural[start:end])
}

func nextClauseBoundary(text string, position int) int {
	if position < 0 {
		position = 0
	}
	if position >= len(text) {
		return len(text)
	}
	if index := strings.IndexAny(text[position:], ",，;；.。!！?？\n"); index >= 0 {
		return position + index
	}
	return len(text)
}

func effectiveNegationOnly(evidence documentEvidence) bool {
	return evidence.negationDetected && (len(evidence.intents) == 0 || len(evidence.actions) == 0)
}

func makeResult(level Level, target AuditTarget, evidence documentEvidence, matches []tokenMatch, metadataExcluded []string, excerpt string) Result {
	matches = deduplicateTokenMatches(matches)
	if excerpt == "" {
		excerpt = buildMatchedExcerpt(evidence.document.text, matches)
	}
	match := &DeterministicRiskMatch{
		RuleID:            RuleCredentialBypassV2,
		RuleVersion:       RuleCredentialBypassV2Version,
		Level:             level,
		TargetKind:        string(target.Kind),
		TargetSource:      string(target.Source),
		MatchedIntent:     valuesFor(matches, evidence.allIntents),
		MatchedTarget:     valuesFor(matches, append(append([]tokenMatch(nil), evidence.allTargets...), evidence.allUnauthorized...)),
		MatchedAction:     valuesFor(matches, evidence.allActions),
		MatchedExcerpt:    truncateRunes(redactSecrets(excerpt), maxMatchedExcerptRunes),
		LexicalTypes:      append([]string(nil), evidence.document.types...),
		NegationDetected:  evidence.negationDetected,
		DefensiveDetected: evidence.defensiveDetected,
		MetadataExcluded:  append([]string(nil), metadataExcluded...),
	}
	result := Result{Level: level, Match: match}
	if level == LevelConfirmed {
		score := 0.95
		result.SuggestedRiskScore = &score
	}
	return result
}

func mergeDiagnosticsEvidence(target, support documentEvidence) documentEvidence {
	target.intents = append(target.intents, support.intents...)
	target.targets = append(target.targets, support.targets...)
	target.unauthorized = append(target.unauthorized, support.unauthorized...)
	target.actions = append(target.actions, support.actions...)
	target.allIntents = append(target.allIntents, support.allIntents...)
	target.allTargets = append(target.allTargets, support.allTargets...)
	target.allUnauthorized = append(target.allUnauthorized, support.allUnauthorized...)
	target.allActions = append(target.allActions, support.allActions...)
	target.document.types = mergeStrings(target.document.types, support.document.types)
	target.negationDetected = target.negationDetected || support.negationDetected
	target.defensiveDetected = target.defensiveDetected || support.defensiveDetected
	return target
}

func linkedContextExcerpt(supports []documentEvidence, target documentEvidence, targetMatches []tokenMatch) string {
	parts := make([]string, 0, len(supports)+1)
	for index := len(supports) - 1; index >= 0; index-- {
		support := supports[index]
		parts = append(parts, buildMatchedExcerpt(support.document.text, candidateMatches(support)))
	}
	parts = append(parts, buildMatchedExcerpt(target.document.text, targetMatches))
	return truncateRunes(strings.TrimSpace(strings.Join(parts, " | ")), maxMatchedExcerptRunes)
}

func collectMetadataExcluded(input Input) []string {
	values := make([]string, 0, 8)
	for _, value := range input.MetadataExcluded {
		if normalized := normalizeMetadataKind(value); normalized != "" {
			values = append(values, normalized)
		}
		if len(values) == 8 {
			break
		}
	}
	for _, context := range input.SupportingContext {
		if context.Source == inputprovenance.SourceTrustedMetadata ||
			(context.MetadataKind != "" && context.MetadataKind != inputprovenance.MetadataNone) {
			if normalized := normalizeMetadataKind(string(context.MetadataKind)); normalized != "" {
				values = append(values, normalized)
			}
		}
	}
	return mergeStrings(nil, values)
}

func normalizeMetadataKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(inputprovenance.MetadataAmbientUI):
		return string(inputprovenance.MetadataAmbientUI)
	case string(inputprovenance.MetadataContextHandoff):
		return string(inputprovenance.MetadataContextHandoff)
	case string(inputprovenance.MetadataEnvironment):
		return string(inputprovenance.MetadataEnvironment)
	default:
		return ""
	}
}

func valuesFor(selected, family []tokenMatch) []string {
	values := make([]string, 0, len(selected))
	for _, match := range selected {
		if containsToken(family, match) {
			values = append(values, truncateRunes(redactSecrets(match.value), 64))
		}
	}
	return mergeStrings(nil, values)
}

func containsToken(values []tokenMatch, target tokenMatch) bool {
	for _, value := range values {
		if value.start == target.start && value.end == target.end && value.label == target.label {
			return true
		}
	}
	return false
}

func spanWidth(matches []tokenMatch) int {
	if len(matches) == 0 {
		return 0
	}
	start, end := matches[0].start, matches[0].end
	for _, match := range matches[1:] {
		if match.start < start {
			start = match.start
		}
		if match.end > end {
			end = match.end
		}
	}
	return end - start
}

func deduplicateTokenMatches(matches []tokenMatch) []tokenMatch {
	result := make([]tokenMatch, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		key := match.label + "\x00" + match.value + "\x00" + strconv.Itoa(match.start) + "\x00" + strconv.Itoa(match.end)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, match)
	}
	return result
}

func mergeStrings(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	result := make([]string, 0, len(existing)+len(incoming))
	for _, values := range [][]string{existing, incoming} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
