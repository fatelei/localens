package app

import (
	"context"
	"localens/internal/catalog"
	"strings"
	"time"
)

// Search returns lexical results immediately, or a merged local semantic result.
func (a *App) Search(query, kind string, semantic bool) (catalog.SearchResult, error) {
	return a.search(query, kind, semantic, semantic, false)
}

// SearchKeywords searches filenames and indexed text without model inference.
func (a *App) SearchKeywords(query, kind string) (catalog.SearchResult, error) {
	return a.search(query, kind, true, false, false)
}

// SearchRelated retrieves model suggestions independently of keyword matches.
func (a *App) SearchRelated(query, kind string) (catalog.SearchResult, error) {
	return a.search(query, kind, true, true, true)
}

func (a *App) search(query, kind string, content, semantic, related bool) (catalog.SearchResult, error) {
	if err := a.ready(); err != nil {
		return catalog.SearchResult{}, err
	}
	key := strings.TrimSpace(query) + "\x00" + kind
	a.mu.Lock()
	if key != a.searchKey {
		a.searchKey = key
		a.searchEpoch++
		if a.contentSearchCancel != nil {
			a.contentSearchCancel()
			a.contentSearchCancel = nil
		}
	}
	epoch := a.searchEpoch
	a.mu.Unlock()
	start := time.Now()
	ctx, cancel := context.WithTimeout(a.ctx, 95*time.Second)
	defer cancel()
	var v []float32
	warning := ""
	if semantic && catalog.SemanticQuery(query, kind) {
		a.mu.Lock()
		ready := a.status.Model == "ready"
		generation := a.modelGeneration
		a.mu.Unlock()
		if ready {
			var err error
			v, err = a.worker.Encode(ctx, "query", "", query)
			if err != nil {
				warning = err.Error()
				a.mu.Lock()
				if a.modelGeneration == generation && a.status.Model == "ready" {
					a.status.ModelError = warning
					if !a.worker.Running() {
						a.status.Model = "error"
					}
				}
				a.mu.Unlock()
			}
		} else {
			warning = "语义搜索尚未启用"
		}
	}
	var result catalog.SearchResult
	var err error
	if !content {
		result, err = a.store.SearchNames(ctx, query, kind)
	} else {
		a.mu.Lock()
		if epoch != a.searchEpoch {
			a.mu.Unlock()
			return catalog.SearchResult{}, context.Canceled
		}
		bodyCtx, bodyCancel := context.WithCancel(ctx)
		if a.contentSearchCancel != nil {
			a.contentSearchCancel()
		}
		a.contentSearchCancel = bodyCancel
		a.mu.Unlock()
		defer bodyCancel()
		if related {
			result, err = a.store.SearchRelated(bodyCtx, query, kind, v)
		} else {
			result, err = a.store.Search(bodyCtx, query, kind, v)
		}
	}
	result.Warning = warning
	result.Elapsed = time.Since(start).Milliseconds()
	return result, err
}
