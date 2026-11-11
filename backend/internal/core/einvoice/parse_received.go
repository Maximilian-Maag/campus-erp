package einvoice

import (
	"encoding/xml"
	"strings"
)

// Reading an inbound document without knowing which syntax it is.
//
// This suite speaks two: UBL 2.1 (what a public body is sent, XRechnung) and UN/CEFACT CII (what a
// supplier's ERP often sends, ZUGFeRD/Factur-X). They are the same invoice in different syntaxes, so
// the caller should not have to decide which one arrived — the root element does.

// ParseReceived reads an inbound invoice in either syntax and validates it with the same rules.
func ParseReceived(doc string) (Invoice, error) {
	root, err := rootElement(doc)
	if err != nil {
		return Invoice{}, ErrMalformed
	}
	switch root {
	case "Invoice":
		return ParseXRechnung(doc)
	case "CrossIndustryInvoice":
		return ParseCII(doc)
	default:
		// A container (a PDF with an embedded invoice, say) or something else entirely: refuse it
		// rather than reading whatever happens to look like a field.
		return Invoice{}, ErrMalformed
	}
}

// rootElement returns the local name of the document's root element, ignoring namespaces and any
// prologue (an XML declaration, a DOCTYPE, comments).
func rootElement(doc string) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(doc))
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local, nil
		}
	}
}
