const sourceURL = 'https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator';

export default function LegalPage() {
  return (
    <div className="max-w-3xl space-y-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-wider text-blue-600">Community Edition</p>
        <h1 className="mt-1 text-2xl font-bold text-slate-900 dark:text-white">Legal and source</h1>
      </div>
      <section className="rounded-lg border border-slate-200 bg-white p-6 text-sm text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300">
        <p>Strata RMM Community Edition is copyright © 2026 Strata Development Platform and contributors.</p>
        <p className="mt-4">
          This program is free software under the GNU Affero General Public License,
          version 3 or later. It is provided without warranty to the extent permitted by law.
        </p>
        <div className="mt-5 flex flex-wrap gap-3">
          <a className="rounded bg-blue-600 px-3 py-2 font-medium text-white hover:bg-blue-700" href={sourceURL} target="_blank" rel="noreferrer">
            Corresponding Source
          </a>
          <a className="rounded border border-slate-300 px-3 py-2 font-medium dark:border-slate-600" href={`${sourceURL}/blob/master/LICENSE`} target="_blank" rel="noreferrer">
            View License
          </a>
        </div>
      </section>
    </div>
  );
}
