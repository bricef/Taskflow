package sqlite

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// mapper holds a cached field mapping between two struct types.
// It is built once on first use and reused for all subsequent calls.
type mapper struct {
	// fields maps source field index → dest field index
	fields []fieldMapping
}

type fieldMapping struct {
	srcIdx  int
	dstIdx  int
	convert func(src reflect.Value) reflect.Value
}

var (
	mapperCache sync.Map // key: [2]reflect.Type → *mapper

	// Known types for conversion.
	tSQLiteBool    = reflect.TypeOf(SQLiteBool(false))
	tTimestamp     = reflect.TypeOf(Timestamp{})
	tNullTimestamp = reflect.TypeOf(NullTimestamp{})
	tStringList    = reflect.TypeOf(StringList{})
	tJSONRaw       = reflect.TypeOf(JSONRaw{})
	tBool          = reflect.TypeOf(false)
	tTime          = reflect.TypeOf(time.Time{})
	tTimePtrType   = reflect.TypeOf((*time.Time)(nil))
	tStringSlice   = reflect.TypeOf([]string{})
	tRawMessage    = reflect.TypeOf(json.RawMessage{})
	tString        = reflect.TypeOf("")
	tIntPtr        = reflect.TypeOf((*int)(nil))
	tStringPtr     = reflect.TypeOf((*string)(nil))
)

func getMapper(src, dst reflect.Type) *mapper {
	key := [2]reflect.Type{src, dst}
	if cached, ok := mapperCache.Load(key); ok {
		return cached.(*mapper)
	}

	m := buildMapper(src, dst)
	mapperCache.Store(key, m)
	return m
}

func buildMapper(srcType, dstType reflect.Type) *mapper {
	var fields []fieldMapping

	for i := 0; i < srcType.NumField(); i++ {
		sf := srcType.Field(i)
		if !sf.IsExported() {
			continue
		}

		df, ok := dstType.FieldByName(sf.Name)
		if !ok {
			panic(fmt.Sprintf("mapper: source field %s.%s has no match in %s", srcType.Name(), sf.Name, dstType.Name()))
		}

		fm := fieldMapping{srcIdx: i, dstIdx: df.Index[0]}

		if sf.Type == df.Type {
			// Identical types — direct copy.
			fields = append(fields, fm)
			continue
		}

		conv := findConversion(sf.Type, df.Type)
		if conv == nil {
			panic(fmt.Sprintf("mapper: no conversion from %s.%s (%s) to %s.%s (%s)",
				srcType.Name(), sf.Name, sf.Type, dstType.Name(), df.Name, df.Type))
		}
		fm.convert = conv
		fields = append(fields, fm)
	}

	// Validate reverse: check that all exported dst fields are covered.
	covered := make(map[string]bool, len(fields))
	for _, fm := range fields {
		covered[dstType.Field(fm.dstIdx).Name] = true
	}
	for i := 0; i < dstType.NumField(); i++ {
		df := dstType.Field(i)
		if !df.IsExported() {
			continue
		}
		if !covered[df.Name] {
			panic(fmt.Sprintf("mapper: dest field %s.%s has no match in %s", dstType.Name(), df.Name, srcType.Name()))
		}
	}

	return &mapper{fields: fields}
}

// typeConversion defines a bidirectional conversion between two types.
type typeConversion struct {
	a    reflect.Type
	b    reflect.Type
	aToB func(reflect.Value) reflect.Value
	bToA func(reflect.Value) reflect.Value
}

var typeConversions = []typeConversion{
	{
		a: tSQLiteBool, b: tBool,
		aToB: func(v reflect.Value) reflect.Value { return reflect.ValueOf(bool(v.Interface().(SQLiteBool))) },
		bToA: func(v reflect.Value) reflect.Value { return reflect.ValueOf(SQLiteBool(v.Bool())) },
	},
	{
		a: tTimestamp, b: tTime,
		aToB: func(v reflect.Value) reflect.Value { return reflect.ValueOf(v.Interface().(Timestamp).Time()) },
		bToA: func(v reflect.Value) reflect.Value { return reflect.ValueOf(Timestamp(v.Interface().(time.Time))) },
	},
	{
		a: tNullTimestamp, b: tTimePtrType,
		aToB: func(v reflect.Value) reflect.Value { return reflect.ValueOf(v.Interface().(NullTimestamp).TimePtr()) },
		bToA: func(v reflect.Value) reflect.Value {
			tp := v.Interface().(*time.Time)
			if tp == nil {
				return reflect.ValueOf(NullTimestamp{})
			}
			return reflect.ValueOf(NullTimestamp{Time: *tp, Valid: true})
		},
	},
	{
		a: tStringList, b: tStringSlice,
		aToB: func(v reflect.Value) reflect.Value { return reflect.ValueOf([]string(v.Interface().(StringList))) },
		bToA: func(v reflect.Value) reflect.Value { return reflect.ValueOf(StringList(v.Interface().([]string))) },
	},
	{
		a: tJSONRaw, b: tRawMessage,
		aToB: func(v reflect.Value) reflect.Value { return reflect.ValueOf(json.RawMessage(v.Interface().(JSONRaw))) },
		bToA: func(v reflect.Value) reflect.Value { return reflect.ValueOf(JSONRaw(v.Interface().(json.RawMessage))) },
	},
}

func findConversion(src, dst reflect.Type) func(reflect.Value) reflect.Value {
	// Check the dispatch table for exact type pairs.
	for _, tc := range typeConversions {
		if src == tc.a && dst == tc.b {
			return tc.aToB
		}
		if src == tc.b && dst == tc.a {
			return tc.bToA
		}
	}

	// String-based named types (e.g., string ↔ Priority, string ↔ Role).
	if src.Kind() == reflect.String && dst.Kind() == reflect.String {
		return func(v reflect.Value) reflect.Value { return v.Convert(dst) }
	}

	// Pointer to named string ↔ pointer to string (e.g., *RefType ↔ *string).
	if src.Kind() == reflect.Pointer && dst.Kind() == reflect.Pointer {
		srcElem := src.Elem()
		dstElem := dst.Elem()
		if srcElem.Kind() == reflect.String && dstElem.Kind() == reflect.String {
			return func(v reflect.Value) reflect.Value {
				if v.IsNil() {
					return reflect.Zero(dst)
				}
				converted := v.Elem().Convert(dstElem)
				ptr := reflect.New(dstElem)
				ptr.Elem().Set(converted)
				return ptr
			}
		}
	}

	return nil
}

func (m *mapper) apply(src, dst reflect.Value) {
	for _, fm := range m.fields {
		sv := src.Field(fm.srcIdx)
		if fm.convert != nil {
			dst.Field(fm.dstIdx).Set(fm.convert(sv))
		} else {
			dst.Field(fm.dstIdx).Set(sv)
		}
	}
}

// toModel converts a row struct to a model struct.
// Field names must match between the two types. Type conversions are
// applied automatically for known pairs (SQLiteBool↔bool, Timestamp↔time.Time, etc.).
// Panics on first use if any field has no match or no known conversion.
func toModel[R any, M any](row R) M {
	var m M
	mp := getMapper(reflect.TypeOf(row), reflect.TypeOf(m))
	mp.apply(reflect.ValueOf(row), reflect.ValueOf(&m).Elem())
	return m
}

// fromModel converts a model struct to a row struct.
// Same rules as toModel but in reverse.
func fromModel[M any, R any](model M) R {
	var r R
	mp := getMapper(reflect.TypeOf(model), reflect.TypeOf(r))
	mp.apply(reflect.ValueOf(model), reflect.ValueOf(&r).Elem())
	return r
}
