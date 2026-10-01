package model

import (
	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

func (i *InviteTemplates) saveGreeting(actor access.Actor, format, key string, grouped, individual bool, taken func(string) bool) (string, []store.Op, error) {
	if format == "" {
		return "", nil, access.Invalid("format is required")
	}
	if len(format) > 200 {
		return "", nil, access.Invalid("format is too long")
	}
	row := store.Row{
		"Name":       format,
		"Format":     format,
		"Grouped":    cells.YesNoCell(grouped),
		"Individual": cells.YesNoCell(individual),
		"Email":      actor.Email,
	}
	if key == "" {
		row["Greeting ID"] = id.New(taken)
		return row["Greeting ID"], []store.Op{store.Insert(greetingsTab, row)}, nil
	}
	have, ok := i.greeting(key)
	if !ok || have.CreatedBy != actor.Email {
		return "", nil, access.Forbidden("you can only edit greetings you created")
	}
	return have.ID, []store.Op{store.Update(greetingsTab, store.Row{"Greeting ID": have.ID}, row)}, nil
}

func (i *InviteTemplates) deleteGreeting(actor access.Actor, key string) ([]store.Op, error) {
	if key == "" {
		return nil, access.Invalid("id is required")
	}
	have, ok := i.greeting(key)
	if !ok || have.CreatedBy != actor.Email {
		return nil, access.Forbidden("you can only delete greetings you created")
	}
	return []store.Op{store.Delete(greetingsTab, store.Row{"Greeting ID": have.ID})}, nil
}
