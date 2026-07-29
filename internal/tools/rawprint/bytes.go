package rawprint

import (
	"strconv"
	"strings"
	"time"
)

// UEL is the Universal Exit Language sequence that opens and closes a PJL
// block. A printer that speaks no PJL prints it as text, which is ugly and
// harmless.
const UEL = "\x1b%-12345X"

// formFeed is what ejects the page. Without it a laser printer holds the sheet
// until the next job arrives and the tech concludes nothing happened.
const formFeed = "\x0c"

// QueryBytes is the read-only enquiry: what are you, and how are you doing. It
// contains no form feed, no ENTER LANGUAGE and no JOB, so it cannot make paper
// move. A test asserts those absences, because that property is the whole
// difference between the two buttons on the page.
func QueryBytes() []byte {
	var b strings.Builder
	b.WriteString(UEL)
	b.WriteString("@PJL INFO ID\r\n")
	b.WriteString("@PJL INFO STATUS\r\n")
	b.WriteString(UEL)
	return []byte(b.String())
}

// TestPageBytes is the page that physically prints. The host, the port and the
// time are on the page so a sheet found on a printer later can be traced back
// to this. The port has to be passed in: a page that names 9100 when it was
// sent to 9101 points the next tech at the wrong place.
func TestPageBytes(host string, port int, now time.Time) []byte {
	var b strings.Builder
	b.WriteString(UEL)
	b.WriteString("@PJL JOB NAME = \"CHIT test page\"\r\n")
	b.WriteString("@PJL ENTER LANGUAGE = PCL\r\n")
	b.WriteString("CHIT raw printer test\r\n")
	b.WriteString("\r\n")
	b.WriteString("This page was sent straight to " + host + " on port " + strconv.Itoa(port) + ".\r\n")
	b.WriteString("No printer driver and no print queue were involved.\r\n")
	b.WriteString("\r\n")
	b.WriteString("If you are reading this on paper, the printer and the network are both fine,\r\n")
	b.WriteString("and any printing problem is in the driver, the queue or the print server.\r\n")
	b.WriteString("\r\n")
	b.WriteString("Sent " + now.Format(time.RFC1123) + "\r\n")
	b.WriteString(formFeed)
	b.WriteString(UEL)
	b.WriteString("@PJL EOJ\r\n")
	b.WriteString(UEL)
	return []byte(b.String())
}
