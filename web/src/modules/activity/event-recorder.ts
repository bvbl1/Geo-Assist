import { activity } from "./api";
import { sessionGeneration } from "../../shared/api/client";
type Operation = { key: string; done: boolean; pending?: Promise<void> };
const operations = new Map<string, Operation>();
export function recordOnce(
  visit: string,
  place: string,
  type: "view" | "select",
  user: string,
): Promise<void> {
  const id = `${sessionGeneration()}:${user}:${visit}:${place}:${type}`;
  let op = operations.get(id);
  if (!op) {
    op = { key: crypto.randomUUID(), done: false };
    operations.set(id, op);
  }
  if (op.done) return Promise.resolve();
  if (op.pending) return op.pending;
  const current = op;
  current.pending = activity
    .record(place, type, current.key)
    .then(() => {
      current.done = true;
    })
    .finally(() => {
      current.pending = undefined;
    });
  if (operations.size > 200) {
    for (const [key, old] of operations) {
      if (old.done && key !== id) {
        operations.delete(key);
        break;
      }
    }
  }
  return current.pending;
}
