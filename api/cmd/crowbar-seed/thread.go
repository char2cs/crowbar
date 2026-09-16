package main

import "context"

const (
	pricingPath      = "src/pricing.ts"
	reviewSideRight  = "right"
	fallbackReviewer = "Seed Reviewer"
)

type threadDTO struct {
	ID       string `json:"id"`
	FilePath string `json:"filePath"`
	Line     int    `json:"line"`
	Author   string `json:"author"`
	Body     string `json:"body"`
}

// seedThread is a review comment anchored by a snippet of the branch source
// rather than a line number, so the anchors survive edits to the fixture.
type seedThread struct {
	filePath string
	anchor   string
	body     string
}

func seedThreads() []seedThread {
	return []seedThread{
		{
			filePath: pricingPath,
			anchor:   "if (items.length === 0) {",
			body: "Good catch on the empty cart, but returning 0 here quietly merges " +
				"two different answers: \"the cart is empty\" and \"every line is free\". " +
				"bulkDiscountRate only compares against thresholds so it happens to be " +
				"safe today, but the next caller that divides by this will not be. " +
				"Can we return null and make the tier lookup handle the empty case?",
		},
		{
			filePath: pricingPath,
			anchor:   "return roundToCents(discounted);",
			body: "applyTax is added in this commit but nothing calls it — grandTotal " +
				"still takes taxRate and then drops it, so every order total is short " +
				"by the tax. This should be `return applyTax(discounted, taxRate)`, and " +
				"the rounding moves inside applyTax where it already lives.",
		},
	}
}

// resolveAuthor asks the daemon whose comments these are, so seeded threads are
// attributed to the developer running the seed instead of a placeholder. The
// identity route needs the worktree-owning chat, which is why this runs
// after the feature chat exists; a failure degrades to a generic name rather
// than failing the seed.
//
// identity is one of spec §4.2's shared-bucket surfaces: it moved off the
// deleted /workspaces/:wsId group onto the flat /v0/chats/:chatId prefix (spec
// §8 step 6), so it is addressed by the feature chat's own id rather than by
// the scope threads still use.
func resolveAuthor(
	ctx context.Context,
	d *daemon,
	chatID string,
) string {
	identity, err := getData[struct {
		Login       string `json:"login"`
		DisplayName string `json:"displayName"`
	}](ctx, d, "resolve identity", flatChatPath(chatID, "/identity"))
	if err != nil {
		return fallbackReviewer
	}
	if identity.DisplayName != "" {
		return identity.DisplayName
	}
	if identity.Login != "" {
		return identity.Login
	}
	return fallbackReviewer
}

// ensureThreads opens the seeded review comments, skipping any a previous run
// already left on the same file and line.
func ensureThreads(
	ctx context.Context,
	d *daemon,
	sc scope,
	author string,
) (int, error) {
	path := sc.path("/threads")
	existing, err := getData[[]threadDTO](ctx, d, "list threads", path)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, want := range seedThreads() {
		opened, err := openThread(ctx, d, path, existing, want, author)
		if err != nil {
			return created, err
		}
		created += opened
	}
	return created, nil
}

func openThread(
	ctx context.Context,
	d *daemon,
	path string,
	existing []threadDTO,
	want seedThread,
	author string,
) (int, error) {
	line, err := lineOf(branchPricingSource, want.anchor)
	if err != nil {
		return 0, err
	}
	if threadExists(existing, want.filePath, line) {
		return 0, nil
	}
	body := map[string]any{
		"filePath":  want.filePath,
		"line":      line,
		"startLine": line,
		"endLine":   line,
		"side":      reviewSideRight,
		"author":    author,
		"isAgent":   false,
		"body":      want.body,
	}
	if _, err := postData[threadDTO](ctx, d, "open thread", path, body); err != nil {
		return 0, err
	}
	return 1, nil
}

func threadExists(
	existing []threadDTO,
	filePath string,
	line int,
) bool {
	for _, t := range existing {
		if t.FilePath == filePath && t.Line == line {
			return true
		}
	}
	return false
}
