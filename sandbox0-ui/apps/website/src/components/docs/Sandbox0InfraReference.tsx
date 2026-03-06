import schema from "@/generated/docs/sandbox0infra-schema.json";

type SchemaEntry = {
  path: string;
  type: string;
  required: boolean;
  default?: string;
  enum?: string[];
  description?: string;
};

type SchemaSection = {
  key: string;
  title: string;
  description?: string;
  entries: SchemaEntry[];
};

type Sandbox0InfraReferenceProps = {
  include?: string;
};

function parseInclude(include?: string): Set<string> | null {
  if (!include) {
    return null;
  }

  const items = include
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

  return items.length > 0 ? new Set(items) : null;
}

function formatDefault(value?: string): string {
  if (!value || value === "{}") {
    return "-";
  }
  return value;
}

function formatDescription(entry: SchemaEntry): string {
  const parts: string[] = [];
  if (entry.description) {
    parts.push(entry.description);
  }
  if (entry.enum && entry.enum.length > 0) {
    parts.push(`Allowed values: ${entry.enum.join(", ")}.`);
  }
  return parts.join(" ");
}

function toFieldCount(entries: SchemaEntry[]): number {
  return Math.max(entries.length - 1, 0);
}

export function Sandbox0InfraReference({ include }: Sandbox0InfraReferenceProps) {
  const includeSet = parseInclude(include);
  const sections = (schema.sections as SchemaSection[]).filter((section) => {
    if (!includeSet) {
      return true;
    }
    return includeSet.has(section.key);
  });

  return (
    <div className="my-8 space-y-5">
      <div className="rounded-none border border-foreground/15 bg-surface px-4 py-3 shadow-pixel-sm">
        <p className="mb-0 text-sm text-muted">
          This reference is generated from the `Sandbox0Infra` CRD schema. It stays aligned with defaults,
          enums, and required fields exposed by the operator, while deployment guidance on this page remains curated.
        </p>
      </div>

      {sections.map((section, index) => (
        <details
          key={section.key}
          open={index < 2}
          className="rounded-none border border-foreground/15 bg-background/30 shadow-pixel-sm"
        >
          <summary className="cursor-pointer list-none px-4 py-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <div className="font-pixel text-sm text-foreground">{section.title}</div>
                <div className="mt-1 font-mono text-xs text-accent">{section.key}</div>
              </div>
              <div className="text-xs uppercase tracking-[0.14em] text-muted">
                {toFieldCount(section.entries)} fields
              </div>
            </div>
            {section.description ? (
              <p className="mb-0 mt-2 text-sm text-muted">{section.description}</p>
            ) : null}
          </summary>

          <div className="border-t border-foreground/10 px-4 py-4">
            <div className="overflow-x-auto">
              <table className="w-full border border-muted/35 text-sm">
                <thead className="bg-surface">
                  <tr>
                    <th className="px-4 py-2 text-left font-pixel text-xs border border-muted/35">Field</th>
                    <th className="px-4 py-2 text-left font-pixel text-xs border border-muted/35">Type</th>
                    <th className="px-4 py-2 text-left font-pixel text-xs border border-muted/35">Required</th>
                    <th className="px-4 py-2 text-left font-pixel text-xs border border-muted/35">Default</th>
                    <th className="px-4 py-2 text-left font-pixel text-xs border border-muted/35">Description</th>
                  </tr>
                </thead>
                <tbody>
                  {section.entries.map((entry) => (
                    <tr key={entry.path}>
                      <td className="px-4 py-2 text-muted border border-muted/30 align-top">
                        <code>{entry.path}</code>
                      </td>
                      <td className="px-4 py-2 text-muted border border-muted/30 align-top">
                        <code>{entry.type}</code>
                      </td>
                      <td className="px-4 py-2 text-muted border border-muted/30 align-top">
                        {entry.required ? "Yes" : "No"}
                      </td>
                      <td className="px-4 py-2 text-muted border border-muted/30 align-top">
                        <code>{formatDefault(entry.default)}</code>
                      </td>
                      <td className="px-4 py-2 text-muted border border-muted/30 align-top">
                        {formatDescription(entry) || "-"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </details>
      ))}
    </div>
  );
}
