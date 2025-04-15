package utils

import (
	"github.com/stretchr/testify/assert"
	"math/rand"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestPtr(t *testing.T) {
	someStr := "hello"
	someInt := 1
	assert.Equal(t, &someStr, Ptr("hello"))
	assert.NotEqual(t, &someStr, Ptr("world"))
	assert.Equal(t, &someInt, Ptr(1))
	assert.NotEqual(t, &someInt, Ptr(2))
}

func TestTernary(t *testing.T) {
	assert.Equal(t, 1, Ternary(true, 1, 2))
	assert.Equal(t, 2, Ternary(false, 1, 2))
	assert.Equal(t, "hello", Ternary(true, "hello", "world"))
	assert.Equal(t, "world", Ternary(false, "hello", "world"))
}

func TestTernaryOrZero(t *testing.T) {
	assert.Equal(t, 1, TernaryOrZero(true, 1))
	assert.Equal(t, 0, TernaryOrZero(false, 1))
	assert.Equal(t, "foo", TernaryOrZero(true, "foo"))
	assert.Equal(t, "", TernaryOrZero(false, "foo"))
}

func TestOr(t *testing.T) {
	assert.Equal(t, "default", Or("", "default"))
	assert.Equal(t, "value", Or("value", "default"))
	assert.Equal(t, 1, Or(0, 1))
	assert.Equal(t, 2, Or(2, 1))
	assert.Equal(t, Ptr(1), Or((*int)(nil), Ptr(1)))
	assert.Equal(t, Ptr(2), Or(Ptr(2), Ptr(1)))
}

func TestDefault(t *testing.T) {
	assert.Equal(t, true, Default((*bool)(nil), true))
	assert.Equal(t, false, Default((*bool)(nil), false))
	assert.Equal(t, true, Default(Ptr(true), false))
	assert.Equal(t, false, Default(Ptr(false), true))
	assert.Equal(t, 1, Default((*int)(nil), 1))
	assert.Equal(t, 2, Default(Ptr(2), 1))
}

func TestFirst(t *testing.T) {
	assert.Equal(t, 1, First(1, 2, 3, 4))
}

func TestSecond(t *testing.T) {
	assert.Equal(t, 2, Second(1, 2, 3, 4))
}

func TestCast(t *testing.T) {
	var origin any = "test1"
	v, ok := Cast[string](origin)
	assert.True(t, ok)
	assert.Equal(t, "test1", v)
	v1, ok1 := Cast[int](origin)
	assert.False(t, ok1)
	assert.Equal(t, 0, v1)
}

func TestCastInto(t *testing.T) {
	origin := reflect.ValueOf("test1")
	var into string
	assert.True(t, CastInto(origin, &into))
	assert.Equal(t, "test1", into)

	origin1 := "test1"
	var into1 string
	assert.True(t, CastInto(origin1, &into1))
	assert.Equal(t, "test1", into1)

	origin2 := "test1"
	var into2 int
	assert.False(t, CastInto(origin2, &into2))
	assert.Equal(t, 0, into2)
}

func TestTryCast(t *testing.T) {
	var i any = Ptr(1)
	assert.True(t, TryCast[*int](i))

	var j any = reflect.ValueOf(Ptr(1))
	assert.True(t, TryCast[*int](j))
	assert.Equal(t, Ptr(1), j.(reflect.Value).Interface())
}

func TestBuildConfig(t *testing.T) {
	type Config struct {
		A, B, C string
	}
	Opt1 := func(c *Config) { c.A = "hello" }
	Opt2 := func(c *Config) { c.B = "world" }
	c := BuildConfig([]func(*Config){Opt1, Opt2})
	assert.Equal(t, &Config{A: "hello", B: "world"}, c)
}

func TestApplyOptions(t *testing.T) {
	type Config struct {
		A, B, C string
	}
	Opt1 := func(c *Config) { c.A = "hello" }
	Opt2 := func(c *Config) { c.B = "world" }
	c := &Config{}
	ApplyOptions(c, []func(*Config){Opt1, Opt2})
	assert.Equal(t, &Config{A: "hello", B: "world"}, c)
}

func TestInsertionSort(t *testing.T) {
	arr := []int{4, 3, 1, 2, 5, 6, 7, 8, 9} // first 4 elements are unsorted
	InsertionSortPartial(arr, 4, func(a, b int) bool { return a < b })
	assert.Equal(t, arr, []int{1, 2, 3, 4, 5, 6, 7, 8, 9})
}

func generateTestData(size int, unsortedPrefix int) []int {
	data := make([]int, size)
	for i := range data {
		data[i] = i
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(unsortedPrefix, func(i, j int) {
		data[i], data[j] = data[j], data[i]
	})
	return data
}

func BenchmarkInsertionSortPartial(b *testing.B) {
	for i := 0; i < b.N; i++ {
		arr := generateTestData(100, 10)
		InsertionSortPartial(arr, 10, func(a, b int) bool { return a < b })
	}
}

func BenchmarkSortSlice(b *testing.B) {
	for i := 0; i < b.N; i++ {
		arr := generateTestData(100, 10)
		sort.Slice(arr, func(i, j int) bool { return arr[i] < arr[j] })
	}
}
