package partition

import "fmt"

// tokenToGoFmt maps the 5 spec format tokens to Go time.Format strings.
var tokenToGoFmt = map[string]string{
	"yyyy":          "2006",
	"yyyy-mm":       "2006-01",
	"yyyy-mm-dd":    "2006-01-02",
	"yyyymmdd":      "20060102",
	"yyyy-mm-dd-hh": "2006-01-02-15",
}

func lookupGoFmt(token string) (string, error) {
	if f, ok := tokenToGoFmt[token]; ok {
		return f, nil
	}
	return "", fmt.Errorf("invalid partition format %q: allowed formats are yyyy, yyyy-mm, yyyy-mm-dd, yyyymmdd, yyyy-mm-dd-hh", token)
}
