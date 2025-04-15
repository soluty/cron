package utils

import (
	"math/rand/v2"
	"reflect"
	"time"
)

// Ptr ...
func Ptr[T any](v T) *T { return &v }

// Ternary ...
func Ternary[T any](predicate bool, a, b T) T {
	if predicate {
		return a
	}
	return b
}

// TernaryOrZero ...
func TernaryOrZero[T any](predicate bool, a T) (zero T) {
	return Ternary(predicate, a, zero)
}

// Or return "a" if it is non-zero otherwise "b"
func Or[T comparable](a, b T) (zero T) {
	return Ternary(a != zero, a, b)
}

// Default return the value of "v" if not nil, otherwise return the default "d" value
func Default[T any](v *T, d T) T {
	if v == nil {
		return d
	}
	return *v
}

// First returns the first argument
func First[T any](a T, _ ...any) T { return a }

// Second returns the second argument
func Second[T any](_ any, a T, _ ...any) T { return a }

// BuildConfig ...
func BuildConfig[C any, F ~func(*C)](opts []F) *C {
	var cfg C
	return ApplyOptions(&cfg, opts)
}

// ApplyOptions ...
func ApplyOptions[C any, F ~func(*C)](cfg *C, opts []F) *C {
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Cast ...
func Cast[T any](origin any) (T, bool) {
	if val, ok := origin.(reflect.Value); ok {
		origin = val.Interface()
	}
	val, ok := origin.(T)
	return val, ok
}

// TryCast ...
func TryCast[T any](origin any) bool {
	_, ok := Cast[T](origin)
	return ok
}

// CastInto ...
func CastInto[T any](origin any, into *T) bool {
	originVal, ok := origin.(reflect.Value)
	if !ok {
		originVal = reflect.ValueOf(origin)
	}
	if originVal.IsValid() {
		if _, ok := originVal.Interface().(T); ok {
			rv := reflect.ValueOf(into)
			if rv.Kind() == reflect.Pointer && !rv.IsNil() {
				rv.Elem().Set(originVal)
				return true
			}
		}
	}
	return false
}

// InsertionSortPartial insertion Sort is particularly fast when:
//   - The slice is already sorted or nearly sorted.
//   - Only a small number of elements (especially near the start) are out of place.
func InsertionSortPartial[T any](data []T, n int, less func(a, b T) bool) {
	// Sort first n elements
	for i := 1; i < n; i++ {
		key := data[i]
		j := i - 1
		for j >= 0 && less(key, data[j]) {
			data[j+1] = data[j]
			j--
		}
		data[j+1] = key
	}
	// Now, push those first n elements into the rest of the (already sorted) slice
	for i := 0; i < n; i++ {
		key := data[i]
		j := i
		for j < len(data)-1 && less(data[j+1], key) {
			data[j] = data[j+1]
			j++
		}
		data[j] = key
	}
}

// EnsureRange ensure min is smaller or equal to max
func EnsureRange(min, max int64) (int64, int64) {
	if max < min {
		min, max = max, min
	}
	return min, max
}

// Random generates a number between min and max inclusively
func Random(min, max int64) int64 {
	if min == max {
		return min
	}
	min, max = EnsureRange(min, max)
	return rand.Int64N(max-min+1) + min
}

// RandDuration generates random duration
func RandDuration(min, max time.Duration) time.Duration {
	n := Random(min.Nanoseconds(), max.Nanoseconds())
	return time.Duration(n)
}
