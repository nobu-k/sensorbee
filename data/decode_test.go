package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecoder(t *testing.T) {
	Convey("Given a decoder with the default config", t, func() {
		d := NewDecoder(nil)

		s := &struct {
			B bool
			I int
			F float64
			S string `bql:"str_key"`
		}{}

		Convey("When decoding a map", func() {
			So(d.Decode(Map{
				"b":       True,
				"i":       Int(10),
				"f":       Float(3.14),
				"str_key": String("str"),
			}, s), ShouldBeNil)

			Convey("Then it should decode a boolean", func() {
				So(s.B, ShouldBeTrue)
			})

			Convey("Then it should decode an integer", func() {
				So(s.I, ShouldEqual, 10)
			})

			Convey("Then it should decode a float", func() {
				So(s.F, ShouldEqual, 3.14)
			})

			Convey("Then it should decode a string", func() {
				So(s.S, ShouldEqual, "str")
			})
		})
	})
}

func TestToSnakeCase(t *testing.T) {
	Convey("toSnakeCase should transform camelcase to snake case", t, func() {
		cases := [][]string{
			{"Test", "test"},
			{"ParseBQL", "parse_bql"},
			{"B2B", "b_2_b"},
		}

		for _, c := range cases {
			So(toSnakeCase(c[0]), ShouldEqual, c[1])
		}
	})
}
