// Copyright 2016 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
//
// Author: Radu Berinde (radu@cockroachlabs.com)

// This file generates batch_generated.go. It can be run via:
//    go run -tags gen-batch gen_batch.go

// +build gen-batch

package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/cockroachdb/cockroach/pkg/roachpb"
)

// reqFieldInfo contains information for each field in RequestUnion.
type reqFieldInfo struct {
	name string

	// responseField is the field in ResponseUnion that corresponds to this
	// request.
	responseField string

	// responseType is the type of the responseField.
	responseType string
}

var fields []reqFieldInfo

func initFields() {
	t := reflect.TypeOf(roachpb.RequestUnion{})

	for i := 0; i < t.NumField(); i++ {
		var info reqFieldInfo
		field := t.Field(i).Name
		info.name = field
		// The ResponseUnion fields match those in RequestUnion, with the
		// following exceptions:
		switch field {
		case "TransferLease":
			field = "RequestLease"
		}
		info.responseField = field
		respField, ok := reflect.TypeOf(roachpb.ResponseUnion{}).FieldByName(field)
		if !ok {
			panic(fmt.Sprintf("invalid response field %s", field))
		}
		info.responseType = respField.Type.Elem().Name()
		fields = append(fields, info)
	}
}

func main() {
	initFields()
	n := len(fields)

	f, err := os.Create("batch_generated.go")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error opening file: ", err)
		os.Exit(1)
	}

	fmt.Fprint(f, `// Code generated by gen_batch.go; DO NOT EDIT

package roachpb

import (
	"bytes"
	"fmt"
	"strconv"
)
`)

	// Generate getReqCounts function.

	fmt.Fprintf(f, `
type reqCounts [%d]int32
`, n)

	fmt.Fprint(f, `
// getReqCounts returns the number of times each
// request type appears in the batch.
func (ba *BatchRequest) getReqCounts() reqCounts {
	var counts reqCounts
	for _, r := range ba.Requests {
		switch {
`)

	for i := 0; i < n; i++ {
		fmt.Fprintf(f, `		case r.%s != nil:
			counts[%d]++
`, fields[i].name, i)
	}

	fmt.Fprintf(f, "%s", `		default:
			panic(fmt.Sprintf("unsupported request: %+v", r))
		}
	}
	return counts
}
`)

	// Generate Summary function.

	// A few shorthands to help make the names more terse.
	shorthands := map[string]string{
		"Delete":      "Del",
		"Range":       "Rng",
		"Transaction": "Txn",
		"Reverse":     "Rev",
		"Admin":       "Adm",
		"Increment":   "Inc",
		"Conditional": "C",
		"Check":       "Chk",
		"Truncate":    "Trunc",
	}

	fmt.Fprintf(f, `
var requestNames = []string{`)
	for i := 0; i < n; i++ {
		name := fields[i].name
		for str, short := range shorthands {
			name = strings.Replace(name, str, short, -1)
		}
		fmt.Fprintf(f, `
	"%s",`, name)
	}
	fmt.Fprint(f, `
}
`)

	// We don't use Fprint to avoid go vet warnings about
	// formatting directives in string.
	fmt.Fprintf(f, "%s", `
// Summary prints a short summary of the requests in a batch.
func (ba *BatchRequest) Summary() string {
	if len(ba.Requests) == 0 {
		return "empty batch"
	}
	counts := ba.getReqCounts()
	var buf struct {
		bytes.Buffer
		tmp [10]byte
	}
	for i, v := range counts {
		if v != 0 {
			if buf.Len() > 0 {
				buf.WriteString(", ")
			}
			buf.Write(strconv.AppendInt(buf.tmp[:0], int64(v), 10))
			buf.WriteString(" ")
			buf.WriteString(requestNames[i])
		}
	}
	return buf.String()
}
`)

	// Generate CreateReply function.

	fmt.Fprint(f, `
// CreateReply creates replies for each of the contained requests, wrapped in a
// BatchResponse. The response objects are batch allocated to minimize
// allocation overhead.
func (ba *BatchRequest) CreateReply() *BatchResponse {
	br := &BatchResponse{}
	br.Responses = make([]ResponseUnion, len(ba.Requests))

	counts := ba.getReqCounts()

`)

	for i := 0; i < n; i++ {
		fmt.Fprintf(f, "	var buf%d []%s\n", i, fields[i].responseType)
	}

	fmt.Fprint(f, `
	for i, r := range ba.Requests {
		switch {
`)

	for i := 0; i < n; i++ {
		fmt.Fprintf(f, `		case r.%[2]s != nil:
			if buf%[1]d == nil {
				buf%[1]d = make([]%[3]s, counts[%[1]d])
			}
			br.Responses[i].%[4]s = &buf%[1]d[0]
			buf%[1]d = buf%[1]d[1:]
`, i, fields[i].name, fields[i].responseType, fields[i].responseField)
	}

	fmt.Fprintf(f, "%s", `		default:
			panic(fmt.Sprintf("unsupported request: %+v", r))
		}
	}
	return br
}
`)

	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "Error closing file: ", err)
		os.Exit(1)
	}
}
