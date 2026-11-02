package httpapi

import (
	"encoding/json"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/liamphmurphy/lazyagents/server/internal/api"
	"github.com/liamphmurphy/lazyagents/server/internal/store"
)

func toUser(u store.User) api.User {
	return api.User{Id: u.ID, Name: u.Name, Email: u.Email, Initials: u.Initials, RoleTitle: u.RoleTitle, AvatarColor: u.AvatarColor}
}

func toUserRef(u store.User) *api.UserRef {
	return &api.UserRef{Id: u.ID, Name: u.Name, Initials: u.Initials, AvatarColor: u.AvatarColor}
}

func toAgent(a store.Agent) api.Agent {
	return api.Agent{
		Id: a.ID, Name: a.Name, Handle: a.Handle, ColorIndex: int(a.ColorIndex),
		Role: a.Role, Specialty: a.Specialty, Model: a.Model, Status: api.AgentStatus(a.Status),
	}
}

func toAgentRef(a store.Agent) *api.AgentRef {
	return &api.AgentRef{Id: a.ID, Name: a.Name, Handle: a.Handle, ColorIndex: int(a.ColorIndex)}
}

func toProject(p store.Project) api.Project {
	return api.Project{Id: p.ID, Key: p.Key, Name: p.Name, Color: p.Color}
}

func toLabel(l store.Label) api.Label {
	return api.Label{Id: l.ID, Name: l.Name, Color: l.Color}
}

func toCriterion(c store.Criterion) api.Criterion {
	return api.Criterion{Id: c.ID, Position: int(c.Position), Text: c.Text, Done: c.Done}
}

func toDate(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	return &openapi_types.Date{Time: *t}
}

func toPR(number *int32, state, url *string, add, del *int32) *api.PullRequest {
	if number == nil {
		return nil
	}
	pr := api.PullRequest{Number: int(*number), State: api.Open, Url: url}
	if state != nil {
		pr.State = api.PullRequestState(*state)
	}
	if add != nil {
		v := int(*add)
		pr.Additions = &v
	}
	if del != nil {
		v := int(*del)
		pr.Deletions = &v
	}
	return &pr
}

// actorDirectory resolves activity actors to display data.
type actorDirectory struct {
	users  map[[16]byte]store.User
	agents map[[16]byte]store.Agent
}

func (d actorDirectory) actor(typ string, id *openapi_types.UUID) api.Actor {
	a := api.Actor{Type: api.ActorType(typ), Id: id, Name: "System"}
	if id == nil {
		return a
	}
	switch typ {
	case "user":
		if u, ok := d.users[*id]; ok {
			a.Name, a.Initials, a.AvatarColor = u.Name, &u.Initials, u.AvatarColor
		} else {
			a.Name = "Deleted user"
		}
	case "agent":
		if ag, ok := d.agents[*id]; ok {
			ci := int(ag.ColorIndex)
			a.Name, a.ColorIndex = ag.Name, &ci
		} else {
			a.Name = "Deleted agent"
		}
	}
	return a
}

func (d actorDirectory) event(e store.ActivityEvent) api.ActivityEvent {
	payload := map[string]any{}
	_ = json.Unmarshal(e.Payload, &payload)
	return api.ActivityEvent{Id: e.ID, Actor: d.actor(e.ActorType, e.ActorID), Verb: e.Verb, Payload: payload, CreatedAt: e.CreatedAt}
}
