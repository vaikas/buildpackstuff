package detect

import "testing"

func TestAllCases(t *testing.T) {

	tests := map[string]*FunctionDetails{
		"./testdata/f1.go": &FunctionDetails{
			Name:      "Receive",
			Signature: "func(event.Event)",
		},
		"./testdata/f2.go": &FunctionDetails{
			Name:      "Receive2",
			Signature: "func(context.Context, event.Event) (*event.Event, protocol.Result)",
		},
		"./testdata/f3.go": &FunctionDetails{
			Name:      "Receive3",
			Signature: "func(context.Context, event.Event) (*event.Event, protocol.Result)",
		},
		"./testdata/f4-bad.go": nil,
		"./testdata/f5.go": &FunctionDetails{
			Name:      "Receive4",
			Signature: "func(context.Context, event.Event) (*event.Event, error)",
		},
		"./testdata/f6.go": &FunctionDetails{
			Name:      "Receive5",
			Signature: "func(event.Event) (*event.Event, error)",
		},
	}

	for file, expected := range tests {
		t.Run(file, func(t *testing.T) {
			got := ReadAndCheckFile(file)
			if expected != nil && got == nil {
				t.Errorf("Expected %+v but got nil", expected)
			} else if expected == nil && got != nil {
				t.Errorf("Expected nil but got %+v", got)
			} else if expected != nil && got != nil {
				if expected.Name != got.Name {
					t.Errorf("Function Name differs got %q expected %q", got.Name, expected.Name)
				}
				if expected.Signature != got.Signature {
					t.Errorf("Signature differs got %q expected %q", got.Signature, expected.Signature)
				}
			}
		})
	}
}
