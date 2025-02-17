package database

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/sourcegraph/log/logtest"
	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/sourcegraph/internal/database/dbtest"
	"github.com/sourcegraph/sourcegraph/internal/types"
)

func TestRolePermissionAssign(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))
	store := db.RolePermissions()

	r, p := createRoleAndPermission(ctx, t, db)

	t.Run("without permission id", func(t *testing.T) {
		err := store.Assign(ctx, AssignRolePermissionOpts{
			RoleID: r.ID,
		})
		require.ErrorContains(t, err, "missing permission id")
	})

	t.Run("without role id", func(t *testing.T) {
		err := store.Assign(ctx, AssignRolePermissionOpts{
			PermissionID: p.ID,
		})
		require.ErrorContains(t, err, "missing role id")
	})

	t.Run("success", func(t *testing.T) {
		err := store.Assign(ctx, AssignRolePermissionOpts{
			RoleID:       r.ID,
			PermissionID: p.ID,
		})
		require.NoError(t, err)

		rp, err := store.GetByRoleIDAndPermissionID(ctx, GetRolePermissionOpts{
			RoleID:       r.ID,
			PermissionID: p.ID,
		})
		require.NoError(t, err)
		require.NotNil(t, rp)
		require.Equal(t, rp.RoleID, r.ID)
		require.Equal(t, rp.PermissionID, p.ID)

		// This shouldn't fail the second time since we're upserting.
		err = store.Assign(ctx, AssignRolePermissionOpts{
			RoleID:       r.ID,
			PermissionID: p.ID,
		})
		require.NoError(t, err)
	})
}

func TestRolePermissionAssignToSystemRole(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))
	store := db.RolePermissions()

	_, p := createRoleAndPermission(ctx, t, db)

	t.Run("without permission id", func(t *testing.T) {
		err := store.AssignToSystemRole(ctx, AssignToSystemRoleOpts{
			Role: types.SiteAdministratorSystemRole,
		})
		require.ErrorContains(t, err, "permission id is required")
	})

	t.Run("without role", func(t *testing.T) {
		err := store.AssignToSystemRole(ctx, AssignToSystemRoleOpts{
			PermissionID: p.ID,
		})
		require.ErrorContains(t, err, "role is required")
	})

	t.Run("success", func(t *testing.T) {
		err := store.AssignToSystemRole(ctx, AssignToSystemRoleOpts{
			PermissionID: p.ID,
			Role:         types.SiteAdministratorSystemRole,
		})
		require.NoError(t, err)

		rps, err := store.GetByPermissionID(ctx, GetRolePermissionOpts{
			PermissionID: p.ID,
		})
		require.NoError(t, err)
		require.NotNil(t, rps)
		require.Len(t, rps, 1)

		// This shouldn't fail the second time since we're upserting.
		err = store.AssignToSystemRole(ctx, AssignToSystemRoleOpts{
			PermissionID: p.ID,
			Role:         types.SiteAdministratorSystemRole,
		})
		require.NoError(t, err)
	})
}

func TestRolePermissionBulkAssignPermissionsToSystemRoles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))
	store := db.RolePermissions()

	_, p := createRoleAndPermission(ctx, t, db)

	t.Run("without permission id", func(t *testing.T) {
		err := store.BulkAssignPermissionsToSystemRoles(ctx, BulkAssignPermissionsToSystemRolesOpts{})
		require.ErrorContains(t, err, "permission id is required")
	})

	t.Run("without roles", func(t *testing.T) {
		err := store.BulkAssignPermissionsToSystemRoles(ctx, BulkAssignPermissionsToSystemRolesOpts{
			PermissionID: p.ID,
		})
		require.ErrorContains(t, err, "roles are required")
	})

	t.Run("success", func(t *testing.T) {
		systemRoles := []types.SystemRole{types.SiteAdministratorSystemRole, types.UserSystemRole}
		err := store.BulkAssignPermissionsToSystemRoles(ctx, BulkAssignPermissionsToSystemRolesOpts{
			PermissionID: p.ID,
			Roles:        systemRoles,
		})
		require.NoError(t, err)

		rps, err := store.GetByPermissionID(ctx, GetRolePermissionOpts{
			PermissionID: p.ID,
		})
		require.NoError(t, err)
		require.NotNil(t, rps)
		require.Len(t, rps, len(systemRoles))

		// This shouldn't fail the second time since we're upserting.
		err = store.BulkAssignPermissionsToSystemRoles(ctx, BulkAssignPermissionsToSystemRolesOpts{
			PermissionID: p.ID,
			Roles:        systemRoles,
		})
		require.NoError(t, err)
	})
}

func TestRolePermissionGetByRoleIDAndPermissionID(t *testing.T) {
	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))
	store := db.RolePermissions()

	r, p := createRoleAndPermission(ctx, t, db)
	err := store.Assign(ctx, AssignRolePermissionOpts{
		RoleID:       r.ID,
		PermissionID: p.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("without permission ID", func(t *testing.T) {
		rp, err := store.GetByRoleIDAndPermissionID(ctx, GetRolePermissionOpts{
			RoleID: r.ID,
		})
		require.Nil(t, rp)
		require.Error(t, err)
		require.Equal(t, err.Error(), "missing permission id")
	})

	t.Run("without role ID", func(t *testing.T) {
		rp, err := store.GetByRoleIDAndPermissionID(ctx, GetRolePermissionOpts{
			PermissionID: p.ID,
		})
		require.Nil(t, rp)
		require.Error(t, err)
		require.Equal(t, err.Error(), "missing role id")
	})

	t.Run("non existent role id and permission id", func(t *testing.T) {
		pid := int32(1083)
		rid := int32(2342)
		rp, err := store.GetByRoleIDAndPermissionID(ctx, GetRolePermissionOpts{
			PermissionID: pid,
			RoleID:       rid,
		})

		require.Nil(t, rp)
		require.Error(t, err)
		require.Equal(t, err, &RolePermissionNotFoundErr{PermissionID: pid, RoleID: rid})
	})

	t.Run("success", func(t *testing.T) {
		rp, err := store.GetByRoleIDAndPermissionID(ctx, GetRolePermissionOpts{
			PermissionID: p.ID,
			RoleID:       r.ID,
		})

		require.NoError(t, err)
		require.Equal(t, rp.RoleID, r.ID)
		require.Equal(t, rp.PermissionID, p.ID)
	})
}

func TestRolePermissionGetByRoleID(t *testing.T) {
	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))
	store := db.RolePermissions()

	r := createTestRoleForRolePermission(ctx, "TEST ROLE", t, db)

	totalRolePermissions := 5
	for i := 1; i <= totalRolePermissions; i++ {
		p := createTestPermissionForRolePermission(ctx, fmt.Sprintf("action-%d", i), t, db)
		err := store.Assign(ctx, AssignRolePermissionOpts{
			RoleID:       r.ID,
			PermissionID: p.ID,
		})

		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("without role ID", func(t *testing.T) {
		rp, err := store.GetByRoleID(ctx, GetRolePermissionOpts{})

		require.Nil(t, rp)
		require.Error(t, err)
		require.Equal(t, err.Error(), "missing role id")
	})

	t.Run("with correct args", func(t *testing.T) {
		rps, err := store.GetByRoleID(ctx, GetRolePermissionOpts{
			RoleID: r.ID,
		})

		require.NoError(t, err)
		require.Len(t, rps, totalRolePermissions)

		for _, rp := range rps {
			require.Equal(t, rp.RoleID, r.ID)
		}
	})
}

func TestRolePermissionGetByPermissionID(t *testing.T) {
	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))
	store := db.RolePermissions()

	p := createTestPermissionForRolePermission(ctx, "READ", t, db)

	totalRolePermissions := 5
	for i := 1; i <= totalRolePermissions; i++ {
		r := createTestRoleForRolePermission(ctx, fmt.Sprintf("TEST ROLE-%d", i), t, db)
		err := store.Assign(ctx, AssignRolePermissionOpts{
			RoleID:       r.ID,
			PermissionID: p.ID,
		})

		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("without permission ID", func(t *testing.T) {
		rp, err := store.GetByPermissionID(ctx, GetRolePermissionOpts{})

		require.Nil(t, rp)
		require.Error(t, err)
		require.Equal(t, err.Error(), "missing permission id")
	})

	t.Run("with correct args", func(t *testing.T) {
		rps, err := store.GetByPermissionID(ctx, GetRolePermissionOpts{
			PermissionID: p.ID,
		})

		require.NoError(t, err)
		require.Len(t, rps, totalRolePermissions)

		for _, rp := range rps {
			require.Equal(t, rp.PermissionID, p.ID)
		}
	})
}

func TestRolePermissionRevoke(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))
	store := db.RolePermissions()

	r, p := createRoleAndPermission(ctx, t, db)

	err := store.Assign(ctx, AssignRolePermissionOpts{
		RoleID:       r.ID,
		PermissionID: p.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("missing permission id", func(t *testing.T) {
		err := store.Revoke(ctx, RevokeRolePermissionOpts{})

		require.Error(t, err)
		require.Equal(t, err.Error(), "missing permission id")
	})

	t.Run("missing role id", func(t *testing.T) {
		err := store.Revoke(ctx, RevokeRolePermissionOpts{
			PermissionID: p.ID,
		})

		require.Error(t, err)
		require.Equal(t, err.Error(), "missing role id")
	})

	t.Run("with non-existent role permission", func(t *testing.T) {
		roleID := int32(1234)
		permissionID := int32(4321)

		err := store.Revoke(ctx, RevokeRolePermissionOpts{
			RoleID:       roleID,
			PermissionID: permissionID,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to revoke role permission")
	})

	t.Run("with existing role permission", func(t *testing.T) {
		err := store.Revoke(ctx, RevokeRolePermissionOpts{
			RoleID:       r.ID,
			PermissionID: p.ID,
		})
		require.NoError(t, err)

		ur, err := store.GetByRoleIDAndPermissionID(ctx, GetRolePermissionOpts{
			RoleID:       r.ID,
			PermissionID: p.ID,
		})
		require.Nil(t, ur)
		require.Error(t, err)
		require.Equal(t, err, &RolePermissionNotFoundErr{
			RoleID:       r.ID,
			PermissionID: p.ID,
		})
	})
}

func TestBulkAssignPermissionsToRole(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))
	store := db.RolePermissions()

	numberOfPerms := 4
	var perms []int32
	for i := 0; i < numberOfPerms; i++ {
		perm := createTestPermissionForRolePermission(ctx, fmt.Sprintf("READ-%d", i), t, db)
		perms = append(perms, perm.ID)
	}

	role := createTestRoleForRolePermission(ctx, "TEST-ROLE", t, db)

	t.Run("without role ID", func(t *testing.T) {
		err := store.BulkAssignPermissionsToRole(ctx, BulkAssignPermissionsToRoleOpts{})
		require.ErrorContains(t, err, "missing role id")
	})

	t.Run("without permissions", func(t *testing.T) {
		err := store.BulkAssignPermissionsToRole(ctx, BulkAssignPermissionsToRoleOpts{
			RoleID: role.ID,
		})
		require.ErrorContains(t, err, "missing permissions")
	})

	t.Run("success", func(t *testing.T) {
		err := store.BulkAssignPermissionsToRole(ctx, BulkAssignPermissionsToRoleOpts{
			RoleID:      role.ID,
			Permissions: perms,
		})
		require.NoError(t, err)

		rps, err := store.GetByRoleID(ctx, GetRolePermissionOpts{RoleID: role.ID})
		require.NoError(t, err)
		require.NotNil(t, rps)
		require.Len(t, rps, numberOfPerms)

		for index, rp := range rps {
			require.Equal(t, rp.RoleID, role.ID)
			require.Equal(t, rp.PermissionID, perms[index])
		}
	})
}

func TestBulkRevokePermissionsForRole(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))
	store := db.RolePermissions()

	role, permission := createRoleAndPermission(ctx, t, db)
	permissionTwo := createTestPermissionForRolePermission(ctx, "READ-1-2", t, db)

	err := store.BulkAssignPermissionsToRole(ctx, BulkAssignPermissionsToRoleOpts{
		RoleID:      role.ID,
		Permissions: []int32{permission.ID, permissionTwo.ID},
	})
	require.NoError(t, err)

	t.Run("without role id", func(t *testing.T) {
		err := store.BulkRevokePermissionsForRole(ctx, BulkRevokePermissionsForRoleOpts{})
		require.ErrorContains(t, err, "missing role id")
	})

	t.Run("without permissions", func(t *testing.T) {
		err := store.BulkRevokePermissionsForRole(ctx, BulkRevokePermissionsForRoleOpts{
			RoleID: role.ID,
		})
		require.ErrorContains(t, err, "missing permissions")
	})

	t.Run("success", func(t *testing.T) {
		err := store.BulkRevokePermissionsForRole(ctx, BulkRevokePermissionsForRoleOpts{
			RoleID:      role.ID,
			Permissions: []int32{permission.ID, permissionTwo.ID},
		})
		require.NoError(t, err)

		rps, err := store.GetByRoleID(ctx, GetRolePermissionOpts{RoleID: role.ID})
		require.NoError(t, err)
		require.Len(t, rps, 0)
	})
}

func TestSyncPermissionsToRole(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := logtest.Scoped(t)
	db := NewDB(logger, dbtest.NewDB(logger, t))
	store := db.RolePermissions()

	role := createTestRoleForRolePermission(ctx, "TEST-ROLE-1", t, db)
	role2 := createTestRoleForRolePermission(ctx, "TEST-ROLE-2", t, db)
	role3 := createTestRoleForRolePermission(ctx, "TEST-ROLE-3", t, db)

	permissionOne := createTestPermissionForRolePermission(ctx, "READ-1-1", t, db)
	permissionTwo := createTestPermissionForRolePermission(ctx, "READ-1-2", t, db)

	err := store.BulkAssignPermissionsToRole(ctx, BulkAssignPermissionsToRoleOpts{
		RoleID:      role.ID,
		Permissions: []int32{permissionOne.ID},
	})
	require.NoError(t, err)

	err = store.BulkAssignPermissionsToRole(ctx, BulkAssignPermissionsToRoleOpts{
		RoleID:      role2.ID,
		Permissions: []int32{permissionOne.ID},
	})
	require.NoError(t, err)

	t.Run("without role id", func(t *testing.T) {
		err := store.SyncPermissionsToRole(ctx, SyncPermissionsToRoleOpts{})
		require.ErrorContains(t, err, "missing role id")
	})

	t.Run("revoke only", func(t *testing.T) {
		err := store.SyncPermissionsToRole(ctx, SyncPermissionsToRoleOpts{
			RoleID:      role.ID,
			Permissions: []int32{},
		})
		require.NoError(t, err)

		rps, err := store.GetByRoleID(ctx, GetRolePermissionOpts{RoleID: role.ID})
		require.NoError(t, err)
		require.Len(t, rps, 0)
	})

	t.Run("assign and revoke", func(t *testing.T) {
		err := store.SyncPermissionsToRole(ctx, SyncPermissionsToRoleOpts{
			RoleID:      role2.ID,
			Permissions: []int32{permissionTwo.ID},
		})
		require.NoError(t, err)

		rps, err := store.GetByRoleID(ctx, GetRolePermissionOpts{RoleID: role2.ID})
		require.NoError(t, err)
		require.Len(t, rps, 1)
		require.Equal(t, rps[0].RoleID, role2.ID)
		require.Equal(t, rps[0].PermissionID, permissionTwo.ID)
	})

	t.Run("assign only", func(t *testing.T) {
		permissions := []int32{permissionOne.ID, permissionTwo.ID}
		err := store.SyncPermissionsToRole(ctx, SyncPermissionsToRoleOpts{
			RoleID:      role3.ID,
			Permissions: permissions,
		})
		require.NoError(t, err)

		rps, err := store.GetByRoleID(ctx, GetRolePermissionOpts{RoleID: role3.ID})
		require.NoError(t, err)
		require.Len(t, rps, 2)

		sort.Slice(rps, func(i, j int) bool {
			return rps[i].PermissionID < rps[j].PermissionID
		})
		for index, rp := range rps {
			require.Equal(t, rp.RoleID, role3.ID)
			require.Equal(t, rp.PermissionID, permissions[index])
		}
	})
}

func createTestPermissionForRolePermission(ctx context.Context, action string, t *testing.T, db DB) *types.Permission {
	t.Helper()
	p, err := db.Permissions().Create(ctx, CreatePermissionOpts{
		Namespace: types.BatchChangesNamespace,
		Action:    action,
	})
	if err != nil {
		t.Fatal(err)
	}

	return p
}

func createRoleAndPermission(ctx context.Context, t *testing.T, db DB) (*types.Role, *types.Permission) {
	t.Helper()
	permission := createTestPermissionForRolePermission(ctx, "READ", t, db)
	role := createTestRoleForRolePermission(ctx, "TEST ROLE", t, db)
	return role, permission
}

func createTestRoleForRolePermission(ctx context.Context, name string, t *testing.T, db DB) *types.Role {
	t.Helper()
	r, err := db.Roles().Create(ctx, name, false)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
