const sourceURL = 'https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator';

export function ProductAttribution({ collapsed }: { collapsed: boolean }) {
  if (collapsed) {
    return (
      <a
        href={sourceURL}
        target="_blank"
        rel="noreferrer"
        className="block px-2 py-2 text-center text-[10px] text-slate-500 hover:text-slate-300"
        title="Strata RMM Community Edition — AGPLv3 source"
      >
        SR
      </a>
    );
  }

  return (
    <div className="px-3 py-2 text-[10px] leading-4 text-slate-500">
      <a href={sourceURL} target="_blank" rel="noreferrer" className="hover:text-slate-300">
        Powered by Strata RMM
      </a>
      <span aria-hidden="true"> · </span>
      <a href="/legal" className="hover:text-slate-300">AGPLv3 · Legal</a>
    </div>
  );
}
