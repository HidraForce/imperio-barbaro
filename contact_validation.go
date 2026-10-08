package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type contactAccount struct {
	PlayerTag  string `json:"playerTag"`
	PlayerName string `json:"playerName"`
	ClanID     string `json:"clanId"`
}

type contact struct {
	Accounts   []contactAccount `json:"accounts"`
	ID         int64            `json:"id"`
	PlayerTag  string           `json:"playerTag,omitempty"`
	PlayerName string           `json:"playerName,omitempty"`
	RealName   string           `json:"realName"`
	Phone      string           `json:"phone"`
	Notes      string           `json:"notes"`
}

var playerTagPattern = regexp.MustCompile(`^#[0289PYLQGRJCUV]{3,15}$`)
var phonePattern = regexp.MustCompile(`^\+?[0-9 ()-]{7,32}$`)

func validateContact(c *contact) error {
	if c.ID < 0 {
		return fmt.Errorf("Identificador de contato inválido.")
	}
	if len(c.Accounts) == 0 && c.PlayerTag != "" {
		c.Accounts = []contactAccount{{PlayerTag: c.PlayerTag, PlayerName: c.PlayerName}}
		if strings.TrimSpace(c.RealName) == "" {
			c.RealName = c.PlayerName
		}
	}
	c.RealName = strings.TrimSpace(c.RealName)
	c.Phone = strings.TrimSpace(c.Phone)
	c.Notes = strings.TrimSpace(c.Notes)
	if c.RealName == "" {
		return fmt.Errorf("Informe o nome ou apelido da pessoa responsável pelas contas.")
	}
	if len(c.Accounts) == 0 || len(c.Accounts) > 20 {
		return fmt.Errorf("Vincule entre 1 e 20 contas ao contato.")
	}
	seen := map[string]bool{}
	for i := range c.Accounts {
		account := &c.Accounts[i]
		account.PlayerTag = "#" + strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(account.PlayerTag)), "#")
		account.PlayerName = strings.TrimSpace(account.PlayerName)
		if !playerTagPattern.MatchString(account.PlayerTag) {
			return fmt.Errorf("Informe uma tag válida para cada conta, como #P0Y8LQ.")
		}
		if seen[account.PlayerTag] {
			return fmt.Errorf("A mesma conta foi informada mais de uma vez.")
		}
		seen[account.PlayerTag] = true
		if account.PlayerName == "" || utf8.RuneCountInString(account.PlayerName) > 100 || !utf8.ValidString(account.PlayerName) {
			return fmt.Errorf("Cada conta precisa de um nome com até 100 caracteres.")
		}
		for _, ch := range account.PlayerName {
			if unicode.IsControl(ch) {
				return fmt.Errorf("O nome da conta contém caracteres inválidos.")
			}
		}
		if account.ClanID != "" && account.ClanID != "1" && account.ClanID != "2" {
			return fmt.Errorf("Escolha o clã 1, o clã 2 ou clã não informado para cada conta.")
		}
	}
	c.PlayerTag = c.Accounts[0].PlayerTag
	c.PlayerName = c.Accounts[0].PlayerName
	for _, field := range []struct {
		value string
		limit int
	}{{c.PlayerName, 100}, {c.RealName, 100}, {c.Notes, 1000}} {
		if !utf8.ValidString(field.value) || utf8.RuneCountInString(field.value) > field.limit {
			return fmt.Errorf("Nome ou observação excede o tamanho permitido.")
		}
		for _, ch := range field.value {
			if unicode.IsControl(ch) && ch != '\n' && ch != '\t' {
				return fmt.Errorf("Os dados contêm caracteres inválidos.")
			}
		}
	}
	if c.Phone != "" {
		if !phonePattern.MatchString(c.Phone) {
			return fmt.Errorf("Informe um telefone válido, com código do país e DDD.")
		}
		digits := 0
		for _, ch := range c.Phone {
			if ch >= '0' && ch <= '9' {
				digits++
			}
		}
		if digits < 7 || digits > 15 {
			return fmt.Errorf("O telefone deve ter entre 7 e 15 dígitos.")
		}
	}
	return nil
}
