/** Work has one URL and one mounted conversation, even when its brief takes focus. */
export type WorkView = "activity" | "tickets" | "epics";
export function workView(params: URLSearchParams): WorkView {
  const view = params.get("view");
  return view === "tickets" || view === "epics" ? view : "activity";
}

export function workHref(
  params: URLSearchParams,
  patch: Record<string, string | null>,
): string {
  const next = new URLSearchParams(params);
  for (const [key, value] of Object.entries(patch)) {
    if (value === null) next.delete(key);
    else next.set(key, value);
  }
  return `/work${next.size ? `?${next}` : ""}`;
}

/** Translate links in shared ticket/run components without remounting the work page. */
export function workTarget(
  pathname: string,
  params: URLSearchParams,
): string | null {
  const run = /^\/runs\/([^/]+)$/.exec(pathname);
  if (run)
    return workHref(params, {
      run: decodeURIComponent(run[1]),
      view: "activity",
      context: null,
    });
  const ticket = /^\/tickets\/([^/]+)$/.exec(pathname);
  if (ticket && ticket[1] !== "new")
    return workHref(params, {
      ticket: decodeURIComponent(ticket[1]),
      ...(workView(params) === "activity"
        ? { context: "ticket" }
        : { view: "tickets", context: null }),
    });
  const epic = /^\/epics\/([^/]+)$/.exec(pathname);
  if (epic)
    return workHref(params, {
      epic: decodeURIComponent(epic[1]),
      ...(workView(params) === "activity"
        ? { context: "epic" }
        : { view: "epics", context: null }),
    });
  return null;
}

/** Read at the event boundary: a preceding native-history update may not have rendered yet. */
export function updateWorkUrl(
  patch: Record<string, string | null>,
  replace = false,
) {
  const href = workHref(new URLSearchParams(window.location.search), patch);
  if (replace) window.history.replaceState(null, "", href);
  else window.history.pushState(null, "", href);
}
