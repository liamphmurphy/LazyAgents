import { CLAUDE, CODEX } from "@/lib/harness-activity";

/** Artwork and motion are the approved footer mockup, on its original 64px grid. */
export function HarnessCharacter({ harness, working }: { harness: string; working: boolean }) {
  const kind = harness === CLAUDE ? "robot" : harness === CODEX ? "comet" : "jet";
  return (
    <span aria-hidden="true" className={`harness-character ${kind} ${working ? "working" : "idle"}`} data-harness={harness}>
      {harness === CLAUDE ? (
        <svg viewBox="0 0 64 64" aria-hidden="true"><ellipse className="shadow" cx="32" cy="58" rx="17" ry="3" fill="#000" opacity=".25"/><g fill="currentColor"><g className="legs"><rect className="leg-a" x="15" y="43" width="6" height="11" rx="2"/><rect className="leg-b" x="25" y="43" width="5" height="12" rx="2"/><rect className="leg-a" x="35" y="43" width="5" height="12" rx="2"/><rect className="leg-b" x="44" y="43" width="6" height="11" rx="2"/></g><g className="body"><rect x="7" y="28" width="7" height="12" rx="3"/><rect x="50" y="28" width="7" height="12" rx="3"/><rect x="12" y="20" width="40" height="27" rx="6"/><path d="M32 20V14" stroke="currentColor" strokeWidth="3"/><circle className="spark" cx="32" cy="12" r="3" fill="#ffd6a0"/><g className="eyes" fill="#30202b"><rect x="22" y="29" width="4" height="6" rx="1.5"/><rect x="39" y="29" width="4" height="6" rx="1.5"/></g><path d="M29 39Q33 42 37 38" fill="none" stroke="#70443d" strokeWidth="1.5" strokeLinecap="round"/></g></g></svg>
      ) : harness === CODEX ? (
        <svg viewBox="0 0 64 64" aria-hidden="true"><ellipse className="shadow" cx="32" cy="58" rx="15" ry="2.5" fill="#000" opacity=".25"/><g className="trail" stroke="currentColor" strokeWidth="3" opacity=".4" strokeLinecap="round"><path d="M8 43L3 50M13 48L9 54M7 35L2 39"/></g><g className="body"><path d="M32 10Q35 24 48 18Q42 29 55 34Q42 35 45 49Q34 42 26 54Q24 40 11 43Q20 32 10 24Q24 25 32 10Z" fill="currentColor"/><g className="eyes" fill="#30213f"><ellipse cx="28" cy="31" rx="2" ry="3"/><ellipse cx="39" cy="31" rx="2" ry="3"/></g><path d="M31 37Q34 40 37 36" fill="none" stroke="#65467f" strokeWidth="1.5" strokeLinecap="round"/></g><g className="orbit" fill="#dec9ff"><circle cx="55" cy="15" r="2"/><path d="M11 9V15M8 12H14" stroke="#dec9ff" strokeWidth="1.5"/></g></svg>
      ) : (
        <svg viewBox="0 0 64 64" aria-hidden="true"><ellipse className="shadow" cx="32" cy="59" rx="15" ry="2.5" fill="#000" opacity=".25"/><g className="body"><g className="exhaust"><path d="M23 45Q20 56 26 59Q31 52 28 45M36 45Q33 56 39 59Q44 52 41 45" fill="#94d5f4"/><path d="M25 45L26 54L28 45M37 45L39 54L41 45" fill="#ffcb9f"/></g><rect x="11" y="26" width="8" height="21" rx="4" fill="#4b85ad"/><rect x="45" y="26" width="8" height="21" rx="4" fill="#4b85ad"/><rect x="18" y="14" width="28" height="34" rx="14" fill="currentColor"/><rect x="21" y="25" width="22" height="13" rx="6" fill="#21485e"/><g className="eyes" fill="#bdeaff"><ellipse cx="28" cy="31" rx="1.8" ry="2.4"/><ellipse cx="37" cy="31" rx="1.8" ry="2.4"/></g><rect x="22" y="44" width="7" height="7" rx="3" fill="currentColor"/><rect x="35" y="44" width="7" height="7" rx="3" fill="currentColor"/><circle cx="32" cy="42" r="1.3" fill="#d0efff"/></g></svg>
      )}
    </span>
  );
}
