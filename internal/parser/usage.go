package parser

// Claude Code writes one API response as several transcript records,
// one per content block (thinking, text, tool_use), and every record
// repeats the response's `usage`. Fable records repeat the final
// figure; Opus 5 records carry the figure as it stood when the block
// was cut, so the first record of a response can say output_tokens 1
// while the last says 342. Summing records therefore overcounts cache
// reads by the block count and can over- or undercount output.
//
// The rule that matches the wire (verified against a cctrace capture
// of 2,607 requests): one usage per response id, the last record wins.

// collapseResponseUsage leaves Usage on the last record of every
// response and clears it on the earlier ones, in place. Records with
// no ResponseID are left alone: they are their own response.
func collapseResponseUsage(messages []*Message) {
	last := make(map[string]*Message)
	for _, msg := range messages {
		if msg == nil || msg.ResponseID == "" || msg.Usage == nil {
			continue
		}
		if prev, ok := last[msg.ResponseID]; ok {
			prev.Usage = nil
		}
		last[msg.ResponseID] = msg
	}
}

// toUsage converts one wire usage into MessageUsage, unpriced.
func (u *usageData) toUsage() *MessageUsage {
	if u == nil {
		return nil
	}
	mu := &MessageUsage{
		InputTokens:       u.InputTokens,
		OutputTokens:      u.OutputTokens,
		CacheReadTokens:   u.CacheReadInputTokens,
		CacheCreateTokens: u.CacheCreationInputTokens,
	}
	if u.CacheCreation != nil {
		mu.CacheCreate1hTokens = u.CacheCreation.Ephemeral1h
	}
	return mu
}

// priceUsage fills in cost under the model's rate card. Priced is
// false when ccx has no row for the model, and the figure is then
// "unknown", never 0.
func priceUsage(mu *MessageUsage, model string) *MessageUsage {
	if mu == nil {
		return nil
	}
	pricing := LookupPricing(model)
	mu.CostUSD = ComputeCost(mu, pricing)
	mu.Priced = pricing != nil
	return mu
}

// responseUsage is the quick-parse counterpart of collapseResponseUsage:
// a single pass keeps the latest usage seen per response id, and the
// roll-up prices each response once. Records without an id are
// counted individually.
type responseUsage struct {
	byID  map[string]responseUsageEntry
	order []string
	loose []responseUsageEntry
}

type responseUsageEntry struct {
	usage *MessageUsage
	model string
}

func (r *responseUsage) set(id string, usage *MessageUsage, model string) {
	if usage == nil {
		return
	}
	if id == "" {
		r.loose = append(r.loose, responseUsageEntry{usage: usage, model: model})
		return
	}
	if r.byID == nil {
		r.byID = make(map[string]responseUsageEntry)
	}
	if _, seen := r.byID[id]; !seen {
		r.order = append(r.order, id)
	}
	r.byID[id] = responseUsageEntry{usage: usage, model: model}
}

func (r *responseUsage) addTo(stats *SessionStats) {
	add := func(e responseUsageEntry) {
		mu := e.usage
		stats.InputTokens += mu.InputTokens
		stats.OutputTokens += mu.OutputTokens
		stats.CacheReadTokens += mu.CacheReadTokens
		stats.CacheCreateTokens += mu.CacheCreateTokens
		if pricing := LookupPricing(e.model); pricing != nil {
			stats.CostUSD += ComputeCost(mu, pricing)
		} else {
			stats.UnpricedTokens += mu.Total()
		}
	}
	for _, id := range r.order {
		add(r.byID[id])
	}
	for _, e := range r.loose {
		add(e)
	}
}
