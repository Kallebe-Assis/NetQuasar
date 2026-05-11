package api

import (
	"errors"
	"strings"
)

func brPhoneDigits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// normalizeBRPhone valida telefone com DDD brasileiro: 10 dígitos (fixo) ou 11 (celular com 9).
func normalizeBRPhone(s string) (string, error) {
	d := brPhoneDigits(s)
	if len(d) != 10 && len(d) != 11 {
		return "", errors.New("telefone deve ter 10 ou 11 dígitos (DDD + número)")
	}
	if d[0] == '0' || d[1] == '0' {
		return "", errors.New("DDD inválido: use o código de área com 2 dígitos (ex.: 11, 85)")
	}
	return d, nil
}
