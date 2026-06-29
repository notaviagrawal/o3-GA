package tools

import (
	"fmt"
	"testing"
)

func TestReturnAllFlags(t *testing.T) {
	flags := ReturnAllFlags("gcc-11")
	actual := len(flags) > 0
	if actual != true {
		t.Error("No flags available in this architect.")
	} else {
		fmt.Printf("Total Flags : %d\n", len(flags))
	}
}
