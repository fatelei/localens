//go:build !darwin || !cgo || !sqlite_fts5

package catalog

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"sync"

	"modernc.org/sqlite"
)

const databaseDriver = "sqlite"

var registerUnicodeLower = sync.OnceValue(func() error {
	if err := sqlite.RegisterDeterministicScalarFunction("fragment_tail", 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		body, _ := args[0].(string)
		if b, ok := args[0].([]byte); ok {
			body = string(b)
		}
		skip, _ := args[1].(int64)
		return []byte(fragmentTail(body, skip)), nil
	}); err != nil {
		return err
	}
	if err := sqlite.RegisterDeterministicScalarFunction("body_fragments", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		body, _ := args[0].(string)
		if b, ok := args[0].([]byte); ok {
			body = string(b)
		}
		return bodyFragmentsJSON(body), nil
	}); err != nil {
		return err
	}
	if err := sqlite.RegisterDeterministicScalarFunction("keyword_rank", 3, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		name, _ := args[0].(string)
		body, _ := args[1].(string)
		query, _ := args[2].(string)
		return keywordRank(name, body, query), nil
	}); err != nil {
		return err
	}
	if err := sqlite.RegisterDeterministicScalarFunction("unicode_lower", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if args[0] == nil {
			return "", nil
		}
		return strings.ToLower(fmt.Sprint(args[0])), nil
	}); err != nil {
		return err
	}
	if err := sqlite.RegisterDeterministicScalarFunction("filename_word_prefix", 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if args[0] != nil && args[1] != nil && filenameWordPrefix(fmt.Sprint(args[0]), fmt.Sprint(args[1])) {
			return int64(1), nil
		}
		return int64(0), nil
	}); err != nil {
		return err
	}
	return sqlite.RegisterDeterministicScalarFunction("unicode_short_tokens", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		body, _ := args[0].(string)
		return shortTokens(body), nil
	})
})
