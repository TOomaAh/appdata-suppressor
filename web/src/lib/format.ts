const UNITS = ["B", "KB", "MB", "GB", "TB"];

export function formatBytes(n: number): string {
  let i = 0;
  while (n >= 1024 && i < UNITS.length - 1) {
    n /= 1024;
    i++;
  }
  return `${i === 0 ? n : n.toFixed(n < 10 ? 1 : 0)} ${UNITS[i]}`;
}

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: "short", timeStyle: "short" });

/** Formats a Go time; its zero value (unknown date) renders as a dash. */
export function formatDate(iso: string): string {
  return iso && !iso.startsWith("0001-") ? dateFormat.format(new Date(iso)) : "—";
}

/** count(3, "folder") === "3 folders" */
export function count(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? "" : "s"}`;
}
