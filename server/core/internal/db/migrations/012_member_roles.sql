-- +migrate Up

-- every member and invitation used to be an owner, because there was no
-- other role to pick. They all become admins, so nobody loses access.
UPDATE organization_members SET role = 'admin' WHERE role = 'owner';
UPDATE organization_invitations SET role = 'admin' WHERE role = 'owner';

-- +migrate Down

-- admins made after the migration become owners too, because the two
-- cannot be told apart.
UPDATE organization_members SET role = 'owner' WHERE role = 'admin';
UPDATE organization_invitations SET role = 'owner' WHERE role = 'admin';
