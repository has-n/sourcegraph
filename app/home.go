package app

import (
	"net/http"
	"net/url"
	"os"

	"golang.org/x/net/context"

	"sourcegraph.com/sqs/pbtypes"
	"src.sourcegraph.com/sourcegraph/app/internal/schemautil"
	"src.sourcegraph.com/sourcegraph/app/internal/tmpl"
	"src.sourcegraph.com/sourcegraph/auth"
	"src.sourcegraph.com/sourcegraph/auth/authutil"
	"src.sourcegraph.com/sourcegraph/go-sourcegraph/sourcegraph"
	"src.sourcegraph.com/sourcegraph/util/httputil/httpctx"
)

func serveHomeDashboard(w http.ResponseWriter, r *http.Request) error {
	ctx := httpctx.FromRequest(r)
	cl, err := sourcegraph.NewClientFromContext(ctx)
	if err != nil {
		return err
	}

	conf, err := cl.Meta.Config(ctx, &pbtypes.Void{})
	if err != nil {
		return err
	}
	rootURL, err := url.Parse(conf.FederationRootURL)
	if err != nil {
		return err
	}

	var listOpts sourcegraph.ListOptions
	if err := schemautil.Decode(&listOpts, r.URL.Query()); err != nil {
		return err
	}

	if listOpts.PerPage == 0 {
		listOpts.PerPage = 50
	}

	repos, err := cl.Repos.List(ctx, &sourcegraph.RepoListOptions{
		Sort:        "pushed",
		Direction:   "desc",
		ListOptions: listOpts,
	})
	if err != nil {
		return err
	}

	return tmpl.Exec(r, w, "home/dashboard.html", http.StatusOK, nil, &struct {
		Repos  []*sourcegraph.Repo
		SGPath string
		Users  []*userInfo
		IsLDAP bool

		RootURL *url.URL

		tmpl.Common
	}{
		Repos:  repos.Repos,
		SGPath: os.Getenv("SGPATH"),
		Users:  getUsersAndInvites(ctx, cl),
		IsLDAP: authutil.ActiveFlags.IsLDAP(),

		RootURL: rootURL,
	})
}

type userInfo struct {
	Identifier string
	Write      bool
	Admin      bool
	Invite     bool
}

func getUsersAndInvites(ctx context.Context, cl *sourcegraph.Client) []*userInfo {
	var users []*userInfo
	ctxActor := auth.ActorFromContext(ctx)
	if !ctxActor.HasAdminAccess() { // current user is not an admin of the instance
		return users
	}

	// Fetch pending invites.
	inviteList, err := cl.Accounts.ListInvites(ctx, &pbtypes.Void{})
	if err == nil {
		for _, invite := range inviteList.Invites {
			users = append(users, &userInfo{
				Identifier: invite.Email,
				Write:      invite.Write,
				Admin:      invite.Admin,
				Invite:     true,
			})
		}
	}

	// Fetch registered users.
	userList, err := cl.Users.List(ctx, &sourcegraph.UsersListOptions{
		ListOptions: sourcegraph.ListOptions{
			PerPage: 10000,
		},
	})
	if err == nil {
		for _, user := range userList.Users {
			users = append(users, &userInfo{
				Identifier: user.Login,
				Write:      user.Write,
				Admin:      user.Admin,
			})
		}
	}
	return users
}
