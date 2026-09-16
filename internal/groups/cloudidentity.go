package groups

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/groupssettings/v1"
	"google.golang.org/api/option"
)

// customer is the heliosian.com Workspace, under which every group is made.
const customer = "customers/C00luxmiy"

const forumLabel = "cloudidentity.googleapis.com/groups.discussion_forum"

// wanted is how every group is set: anyone on the internet may post, since
// the members are outside the Workspace and so is much of what they mail
// about; members from outside the Workspace are allowed, the members being
// heliosschool.org addresses; every member sees the members and the
// messages; nobody leaves by hand, since the rules decide who is on it; and
// nobody joins by asking. The Groups Settings API holds these; the Cloud
// Identity API holds only the group and its members.
var wanted = groupssettings.Groups{
	WhoCanPostMessage:          "ANYONE_CAN_POST",
	MessageModerationLevel:     "MODERATE_NONE",
	AllowExternalMembers:       "true",
	WhoCanViewMembership:       "ALL_MEMBERS_CAN_VIEW",
	WhoCanViewGroup:            "ALL_MEMBERS_CAN_VIEW",
	WhoCanLeaveGroup:           "NONE_CAN_LEAVE",
	WhoCanJoin:                 "INVITED_CAN_JOIN",
	IncludeInGlobalAddressList: "true",
}

// CloudIdentity is Google through the Cloud Identity Groups API and the
// Groups Settings API, as the runtime identity, which holds the Groups Admin
// role in the Workspace.
type CloudIdentity struct {
	svc      *cloudidentity.Service
	settings *groupssettings.Service
}

func NewCloudIdentity(ctx context.Context) (*CloudIdentity, error) {
	svc, err := cloudidentity.NewService(ctx, option.WithScopes(cloudidentity.CloudIdentityGroupsScope))
	if err != nil {
		return nil, err
	}
	settings, err := groupssettings.NewService(ctx, option.WithScopes(groupssettings.AppsGroupsSettingsScope))
	if err != nil {
		return nil, err
	}
	return &CloudIdentity{svc: svc, settings: settings}, nil
}

// settle brings a group's settings to wanted, patching only what differs.
func (c *CloudIdentity) settle(ctx context.Context, address string) error {
	current, err := c.settings.Groups.Get(address).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("read the settings of %s: %w", address, err)
	}
	patch := &groupssettings.Groups{}
	changed := false
	for _, s := range []struct {
		want, have string
		set        *string
	}{
		{wanted.WhoCanPostMessage, current.WhoCanPostMessage, &patch.WhoCanPostMessage},
		{wanted.MessageModerationLevel, current.MessageModerationLevel, &patch.MessageModerationLevel},
		{wanted.AllowExternalMembers, current.AllowExternalMembers, &patch.AllowExternalMembers},
		{wanted.WhoCanViewMembership, current.WhoCanViewMembership, &patch.WhoCanViewMembership},
		{wanted.WhoCanViewGroup, current.WhoCanViewGroup, &patch.WhoCanViewGroup},
		{wanted.WhoCanLeaveGroup, current.WhoCanLeaveGroup, &patch.WhoCanLeaveGroup},
		{wanted.WhoCanJoin, current.WhoCanJoin, &patch.WhoCanJoin},
		{wanted.IncludeInGlobalAddressList, current.IncludeInGlobalAddressList, &patch.IncludeInGlobalAddressList},
	} {
		if s.have != s.want {
			*s.set = s.want
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if _, err := c.settings.Groups.Patch(address, patch).Context(ctx).Do(); err != nil {
		return fmt.Errorf("set the settings of %s: %w", address, err)
	}
	return nil
}

// notFound reads the API's two ways of saying a group or membership is not
// there: a 404, and the 403 it answers a lookup of a missing group with -
// "Error(2028): Permission denied for resource ... (or it may not exist)" -
// which gcloud reads the same way. A real permission failure shows on the
// create that follows, which answers a 403 of its own.
func notFound(err error) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == 404 || (apiErr.Code == 403 && strings.Contains(apiErr.Message, "Error(2028)"))
}

// lookup is a group's resource name by its address, "" for none.
func (c *CloudIdentity) lookup(ctx context.Context, address string) (string, error) {
	resp, err := c.svc.Groups.Lookup().GroupKeyId(address).Context(ctx).Do()
	if notFound(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("look up %s: %w", address, err)
	}
	return resp.Name, nil
}

func (c *CloudIdentity) Ensure(ctx context.Context, address, title, description string) error {
	name, err := c.lookup(ctx, address)
	if err != nil {
		return err
	}
	if name == "" {
		op, err := c.svc.Groups.Create(&cloudidentity.Group{
			GroupKey: &cloudidentity.EntityKey{Id: address}, Parent: customer,
			DisplayName: title, Description: description, Labels: map[string]string{forumLabel: ""},
		}).InitialGroupConfig("EMPTY").Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("create %s: %w", address, err)
		}
		if op.Error != nil {
			return fmt.Errorf("create %s: %s", address, op.Error.Message)
		}
		for attempt := 0; name == ""; attempt++ {
			if attempt == 10 {
				return fmt.Errorf("create %s: the group has not appeared", address)
			}
			time.Sleep(time.Second)
			if name, err = c.lookup(ctx, address); err != nil {
				return err
			}
		}
		return c.settle(ctx, address)
	}
	group, err := c.svc.Groups.Get(name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get %s: %w", address, err)
	}
	if group.DisplayName != title || group.Description != description {
		if _, err := c.svc.Groups.Patch(name, &cloudidentity.Group{DisplayName: title, Description: description}).UpdateMask("displayName,description").Context(ctx).Do(); err != nil {
			return fmt.Errorf("rename %s: %w", address, err)
		}
	}
	return c.settle(ctx, address)
}

func (c *CloudIdentity) Members(ctx context.Context, address string) ([]string, error) {
	name, err := c.lookup(ctx, address)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("no group %s", address)
	}
	out := []string{}
	err = c.svc.Groups.Memberships.List(name).View("BASIC").Pages(ctx, func(page *cloudidentity.ListMembershipsResponse) error {
		for _, m := range page.Memberships {
			if m.PreferredMemberKey != nil {
				out = append(out, strings.ToLower(m.PreferredMemberKey.Id))
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list members of %s: %w", address, err)
	}
	return out, nil
}

func (c *CloudIdentity) Add(ctx context.Context, address, email string) error {
	name, err := c.lookup(ctx, address)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("no group %s", address)
	}
	_, err = c.svc.Groups.Memberships.Create(name, &cloudidentity.Membership{
		PreferredMemberKey: &cloudidentity.EntityKey{Id: email},
		Roles:              []*cloudidentity.MembershipRole{{Name: "MEMBER"}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("add %s to %s: %w", email, address, err)
	}
	return nil
}

func (c *CloudIdentity) Remove(ctx context.Context, address, email string) error {
	name, err := c.lookup(ctx, address)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("no group %s", address)
	}
	membership, err := c.svc.Groups.Memberships.Lookup(name).MemberKeyId(email).Context(ctx).Do()
	if notFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look up %s in %s: %w", email, address, err)
	}
	if _, err := c.svc.Groups.Memberships.Delete(membership.Name).Context(ctx).Do(); err != nil {
		return fmt.Errorf("remove %s from %s: %w", email, address, err)
	}
	return nil
}

func (c *CloudIdentity) Delete(ctx context.Context, address string) error {
	name, err := c.lookup(ctx, address)
	if err != nil {
		return err
	}
	if name == "" {
		return nil
	}
	if _, err := c.svc.Groups.Delete(name).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete %s: %w", address, err)
	}
	return nil
}

func (c *CloudIdentity) Addresses(ctx context.Context) ([]string, error) {
	out := []string{}
	query := fmt.Sprintf("parent=='%s' && '%s' in labels", customer, forumLabel)
	err := c.svc.Groups.Search().Query(query).View("BASIC").Pages(ctx, func(page *cloudidentity.SearchGroupsResponse) error {
		for _, g := range page.Groups {
			if g.GroupKey != nil && strings.HasSuffix(strings.ToLower(g.GroupKey.Id), "@"+Domain) {
				out = append(out, strings.ToLower(g.GroupKey.Id))
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list the groups: %w", err)
	}
	return out, nil
}
