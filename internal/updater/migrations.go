package updater

// The migration compatibility policy (C5-18 §6) — the machine's central
// data-safety gates, evaluated at S6 BEFORE any data touch and BEFORE
// activation (T7's structural guarantee). Pure functions: a validation
// failure touches nothing by construction.

// Reversibility classes (C5-18 §6.1's honestly-declared classes). A migration
// declaring an unknown class is refused — the declaration must be one the
// machine understands, never improvised.
const (
	ReversibilityLossless = "reversible-lossless"
	ReversibilityForward  = "forward-only"
	ReversibilityLossy    = "forward-only-lossy"
)

// ValidateMigrations enforces §6.2's two gates and the declaration sanity,
// refusing BEFORE any data touch (R10/L7):
//
//  1. the min_supported_client gate: installed < min_supported_client →
//     refuse naming the intermediate-upgrade instruction (the machine never
//     multi-hops silently);
//  2. the rollback-compatibility gate: the release must DECLARE that the old
//     binary tolerates the post-migration schema (the delegated REFUSE
//     policy, C5-18 §14) — automatic binary rollback (S9) is then always
//     safe in v1;
//  3. the declaration sanity: every step declares a known reversibility
//     class; the declared schema range is forward (To >= From); a schema
//     advance must declare steps.
//
// The release_version participates in gate 1 (the version the client would
// become); installedVersion is the currently installed release version.
func ValidateMigrations(manifest ReleaseManifest, installedVersion string) error {
	// Gate 1: min_supported_client (C5-17 §4.2's slot, consumed by C5-18 §6.2).
	if manifest.MinSupportedClient != "" {
		if CompareVersions(installedVersion, manifest.MinSupportedClient) < 0 {
			return fail(ReasonMigrationGate,
				"installed version %s is below the release's min_supported_client %s — update through that intermediate first",
				installedVersion, manifest.MinSupportedClient)
		}
	}
	// Gate 2: the rollback-compatibility declaration (§6.3's rule; the
	// delegated policy is REFUSE — C5-18 §14). A release whose migrations
	// would strand the old binary on the post-migration schema is refused in
	// v1 so S9's automatic binary rollback is ALWAYS safe.
	if !manifest.Migrations.RollbackCompatible {
		return fail(ReasonMigrationGate,
			"the release does not declare old-binary/post-migration-schema rollback compatibility — refused in v1 (C5-18 §6.3)")
	}
	// Declaration sanity: the range is forward; a schema advance declares steps.
	if manifest.Migrations.ToSchema < manifest.Migrations.FromSchema {
		return fail(ReasonMigrationGate,
			"migration range %d→%d goes backwards", manifest.Migrations.FromSchema, manifest.Migrations.ToSchema)
	}
	if manifest.Migrations.ToSchema > manifest.Migrations.FromSchema && len(manifest.Migrations.Steps) == 0 {
		return fail(ReasonMigrationGate, "the release advances the schema %d→%d but declares no migrations",
			manifest.Migrations.FromSchema, manifest.Migrations.ToSchema)
	}
	for i, s := range manifest.Migrations.Steps {
		switch s.ReversibilityClass {
		case ReversibilityLossless, ReversibilityForward, ReversibilityLossy:
			// Declared honestly; the class is carried into the update record.
		case "":
			return fail(ReasonMigrationGate, "migration step %d (%q) declares no reversibility class", i, s.ID)
		default:
			return fail(ReasonMigrationGate, "migration step %d (%q) declares unknown reversibility class %q", i, s.ID, s.ReversibilityClass)
		}
		if s.ID == "" {
			return fail(ReasonMigrationGate, "migration step %d carries no identity", i)
		}
	}
	return nil
}
