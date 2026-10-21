package catalog

import (
	"context"
	"errors"
	"time"
)

func (s *Store) startContentWork() {
	ctx, cancel := context.WithCancel(context.Background())
	s.contentCancel = cancel
	s.contentWG.Add(1)
	go func() {
		defer s.contentWG.Done()
		for {
			if ctx.Err() != nil {
				return
			}
			n, err := s.indexTextBatch(ctx)
			if err == nil && n == 0 && OCRAvailable() {
				worked, e := s.runImageOCR(ctx, recognizeImage)
				err = e
				if worked {
					n = 1
				}
			}
			if ctx.Err() == nil {
				s.contentMu.Lock()
				s.contentError = ""
				if err != nil {
					s.contentError = err.Error()
				}
				s.contentMu.Unlock()
			}
			delay := 25 * time.Millisecond
			if n == 0 || err != nil {
				delay = time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

// SetOCREnabled controls future recognition and cancels the current result when paused.
// Existing recognized text and vectors remain indexed.
func (s *Store) SetOCREnabled(enabled bool) {
	s.ocrMu.Lock()
	defer s.ocrMu.Unlock()
	s.ocrEnabled = enabled
	if !enabled && s.ocrCancel != nil {
		s.ocrCancel()
	}
}

func (s *Store) runImageOCR(ctx context.Context, recognize func(context.Context, string) (string, error)) (bool, error) {
	s.ocrMu.Lock()
	if !s.ocrEnabled {
		s.ocrMu.Unlock()
		return false, nil
	}
	requestCtx, cancel := context.WithCancel(ctx)
	s.ocrCancel = cancel
	s.ocrMu.Unlock()
	worked, err := s.processImageText(requestCtx, recognize)
	cancel()
	s.ocrMu.Lock()
	s.ocrCancel = nil
	s.ocrMu.Unlock()
	if ctx.Err() == nil && errors.Is(err, context.Canceled) {
		return false, nil
	}
	return worked, err
}
