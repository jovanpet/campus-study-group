# campus-study-group

Go API deployed to Heroku (`campus-study-group`), with Postgres migrations managed by [goose](https://github.com/pressly/goose).

## Auth

Users sign in with Apple. `POST /auth/apple` takes the app's identity token and authorization code and returns an API token, sent on every other request as `Authorization: Bearer <token>`.

The user row is created at first sign-in. Until the profile is completed with `PUT /me/profile`, responses carry `"onboarded": false`, so the app can send a user who quit mid-onboarding back to the profile form.

| Route | |
|---|---|
| `POST /auth/apple` | Sign in or sign up |
| `POST /auth/logout` | Invalidate the current token |
| `GET /me` | Current user |
| `PUT /me/profile` | Finish onboarding or edit the profile |
| `DELETE /me` | Revoke the Apple grant and delete the account |
| `GET /schools`, `GET /majors` | Options for the profile form |

API tokens last 90 days from last use; only their hash is stored. `SESSION_SECRET` encrypts the stored Apple refresh tokens, so rotating it means existing accounts are deleted without the Apple-side revoke.

For how the flow works end to end, see [docs/auth.md](docs/auth.md). To test without an Apple device (dev sign-in, test helpers), see [docs/testing-auth.md](docs/testing-auth.md).

## Database migrations

Migrations live in `internal/store/migrations/` and are embedded in the binary. They run automatically on every Heroku release (`release: bin/api migrate` in the `Procfile`). To run them locally (uses `DATABASE_URL` from `.env`):

```sh
make migrate
```

To create a new migration:

```sh
make migration name=add_something
```

### Never edit a migration that has already run

goose records each migration's version number and never runs that version again. If you edit a file after it has been applied, the database that already ran it **won't change**, and goose will report `no migrations to run` while the schema no longer matches the file. CI won't catch it either, because the test database is built fresh from the current files.

Always add a **new** migration to change the schema (`ALTER TABLE ...`), even to fix a mistake in an earlier one.

## Resetting the database

This rebuilds a database from scratch using the current migration files. **It deletes all data.** Only do this while there is no real user data. Once there is, write a new migration instead.

### Production (Heroku)

```sh
# 1. Check whether there's anything to lose
heroku pg:psql -a campus-study-group -c "select count(*) from users;"

# 2. Back up first
heroku pg:backups:capture -a campus-study-group

# 3. Wipe the database (drops every table, including goose's version table).
#    Keeps the same add-on and DATABASE_URL, so Terraform and config vars are unaffected.
heroku pg:reset DATABASE_URL -a campus-study-group --confirm campus-study-group

# 4. Rebuild it from the current migrations
heroku run bin/api migrate -a campus-study-group

# 5. Restart so the app reconnects cleanly
heroku ps:restart -a campus-study-group
```

Then check that the schema matches the migration files:

```sh
heroku pg:psql -a campus-study-group -c "\d users"
```

Don't destroy the Postgres add-on through Terraform for this. It would provision a new database with a new `DATABASE_URL`.

### Local

```sh
dropdb -h localhost -U postgres campus_study_group
createdb -h localhost -U postgres campus_study_group
make migrate
```

## Deploys

Every push to `main` runs CI (vet, test, lint) and then a `deploy` job that waits for approval on the `production` environment. Approve it from the run page using **Review deployments**.

If the release command fails, production stays on the previous release.

- **Failed because of something outside the code** (missing config var, database issue): fix it on Heroku, then click **Re-run failed jobs**. The re-run releases the existing build again, so no new commit is needed.
- **Failed because of a code or migration bug**: push a new commit with the fix.
