package console

import (
	"fmt"
	"testing"
)

func TestDebugParse(t *testing.T) {
	BuildColorMap()
	res := parseTviewStyleToANSI("cyan::B")
	fmt.Printf("DEBUG: parseTviewStyleToANSI('cyan::B') = %q\n", res)

	res2 := parseTviewStyleToANSI("red::HD")
	fmt.Printf("DEBUG: parseTviewStyleToANSI('red::HD') = %q\n", res2)
}
