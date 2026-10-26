package lookup

import (
	"errors"
	"testing"
)

func TestNewValue(t *testing.T) {
	v, err := NewValue(" vat_category ", " S ", " Regelsteuersatz ", " Standard rate ", 10, true)
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	if v.Namespace != "vat_category" || v.Code != "S" || v.ShortText != "Regelsteuersatz" ||
		v.LongText != "Standard rate" || v.SortOrder != 10 || !v.Active {
		t.Fatalf("value = %+v", v)
	}

	cases := []struct {
		name                            string
		namespace, code, short, longTxt string
		want                            error
	}{
		{"no namespace", "  ", "S", "x", "", ErrNamespaceRequired},
		{"no code", "ns", " ", "x", "", ErrCodeRequired},
		{"no short text", "ns", "S", "  ", "", ErrShortTextRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewValue(c.namespace, c.code, c.short, c.longTxt, 0, true); !errors.Is(err, c.want) {
				t.Fatalf("NewValue = %v, want %v", err, c.want)
			}
		})
	}
}

func TestStandardCatalogues(t *testing.T) {
	vals := standardCatalogues()
	if len(vals) < 30 {
		t.Fatalf("catalogues = %d, want >= 30", len(vals))
	}
	seen := map[string]bool{}
	namespaces := map[string]int{}
	for _, v := range vals {
		if _, err := NewValue(v.Namespace, v.Code, v.ShortText, v.LongText, v.SortOrder, v.Active); err != nil {
			t.Fatalf("invalid seed value %+v: %v", v, err)
		}
		key := v.Namespace + "/" + v.Code
		if seen[key] {
			t.Fatalf("duplicate seed value %s", key)
		}
		seen[key] = true
		namespaces[v.Namespace]++
	}
	for _, ns := range []string{"document_type", "payment_means", "vat_category", "unit_of_measure", "currency", "country"} {
		if namespaces[ns] == 0 {
			t.Fatalf("namespace %s missing from the seed", ns)
		}
	}
}
