package alter_test

import (
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/alter"
)

// repoTarget builds the RepoTarget that the Process functions take.
func repoTarget(client *api.RESTClient, owner, name string, hasRepo bool) alter.RepoTarget {
	return alter.RepoTarget{Client: client, Owner: owner, Name: name, HasRepo: hasRepo}
}
