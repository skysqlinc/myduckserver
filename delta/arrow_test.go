// Copyright 2025 SkySQL, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package delta_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/marcboeker/go-duckdb"
)

/*
Example is as demonstration of how to read the data from an Arrow view.

This can be very useful when debugging the DeltaAppender.
For that, after a call to DeltaController.prepareArrowView(), you can simply do:

```go

	viewRows, err := tx.QueryContext(ctx, "SELECT * FROM "+viewName)
	if err != nil {
	  return err
	}
	defer viewRows.Close()

	err = printRows(viewRows)
	if err != nil {
	  return err
	}

```
*/
func Example() {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = db.Close()
	}()

	conn, err := db.Conn(context.Background())
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = conn.Close()
	}()

	err = conn.Raw(func(driverConn any) error {
		innerConn, ok := driverConn.(driver.Conn)
		if !ok {
			return fmt.Errorf("driverConn is not a driver.Conn")
		}

		ar, err := duckdb.NewArrowFromConn(innerConn)
		if err != nil {
			return err
		}

		pool := memory.NewGoAllocator()
		schema := arrow.NewSchema(
			[]arrow.Field{
				{Name: "f1_i32", Type: arrow.PrimitiveTypes.Int32},
				{Name: "f2_f64", Type: arrow.PrimitiveTypes.Float64},
				{Name: "f3_str", Type: arrow.BinaryTypes.String},
			},
			nil,
		)

		b := array.NewRecordBuilder(pool, schema)
		defer b.Release()

		b.Field(0).(*array.Int32Builder).AppendValues([]int32{1, 2, 3}, nil)
		b.Field(0).(*array.Int32Builder).AppendValues([]int32{4, 5}, []bool{false, true})
		b.Field(1).(*array.Float64Builder).AppendValues([]float64{1, 2, 3, 4, 5}, nil)
		b.Field(2).(*array.StringBuilder).AppendValues([]string{"a", "b", "c", "d", "e"}, nil)

		rec1 := b.NewRecord()
		defer rec1.Release()

		rr, err := array.NewRecordReader(schema, []arrow.Record{rec1})
		if err != nil {
			return err
		}
		defer rr.Release()

		release, err := ar.RegisterView(rr, "arrow_table")
		if err != nil {
			return err
		}
		defer release()

		// Query the table to verify the data.
		rows, err := db.QueryContext(context.Background(), `SELECT * FROM arrow_table`)
		if err != nil {
			return err
		}
		defer rows.Close()

		err = printRows(rows)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		panic(err)
	}

	// Output:
	// f1_i32	f2_f64	f3_str
	// [1]	[1]	[a]
	// [2]	[2]	[b]
	// [3]	[3]	[c]
	// NULL	[4]	[d]
	// [5]	[5]	[e]
	//
}

func printRows(rows *sql.Rows) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	colNum := len(columns)
	values := make([]interface{}, colNum)
	for i, col := range columns {
		if i > 0 {
			fmt.Printf("\t")
		}
		values[i] = new(sql.RawBytes)
		fmt.Printf("%s", col)
	}

	for rows.Next() {
		err = rows.Scan(values...)
		if err != nil {
			return err
		}
		fmt.Println()
		for i := range columns {
			if i > 0 {
				fmt.Printf("\t")
			}
			if len(*values[i].(*sql.RawBytes)) == 0 {
				fmt.Printf("NULL")
			} else {
				fmt.Printf("[%s]", string(*values[i].(*sql.RawBytes)))
			}
		}
	}

	return nil
}
