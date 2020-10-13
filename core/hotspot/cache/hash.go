package cache

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"hash/fnv"
	"math"
	"reflect"
)

func sum(k interface{}) uint64 {
	switch h := k.(type) {
	case int:
		return hashU64(uint64(h))
	case int8:
		return hashU64(uint64(h))
	case int16:
		return hashU64(uint64(h))
	case int32:
		return hashU64(uint64(h))
	case int64:
		return hashU64(uint64(h))
	case uint:
		return hashU64(uint64(h))
	case uint8:
		return hashU64(uint64(h))
	case uint16:
		return hashU64(uint64(h))
	case uint32:
		return hashU64(uint64(h))
	case uint64:
		return hashU64(h)
	case uintptr:
		return hashU64(uint64(h))
	case float32:
		return hashU64(uint64(math.Float32bits(h)))
	case float64:
		return hashU64(math.Float64bits(h))
	case bool:
		if h {
			return 1
		}
		return 0
	case string:
		return hashString(h)
	}
	if h, ok := hashPointer(k); ok {
		return h
	}
	if h, ok := hashWithGob(k); ok {
		return h
	}
	return 0

}

func hashU64(data uint64) uint64 {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, data)
	return hashByteArray(b)
}

func hashString(data string) uint64 {
	return hashByteArray([]byte(data))
}

func hashWithGob(data interface{}) (uint64, bool) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(data)
	if err != nil {
		return 0, false
	}
	return hashByteArray(buf.Bytes()), true
}

func hashByteArray(bytes []byte) uint64 {
	f := fnv.New64()
	_, err := f.Write(bytes)
	if err != nil {
		return 0
	}
	return binary.LittleEndian.Uint64(f.Sum(nil))
}

func hashPointer(k interface{}) (uint64, bool) {
	v := reflect.ValueOf(k)
	switch v.Kind() {
	case reflect.Ptr, reflect.UnsafePointer, reflect.Func, reflect.Slice, reflect.Map, reflect.Chan:
		return hashU64(uint64(v.Pointer())), true
	default:
		return 0, false
	}
}
