// The workspace's placeholder until notes open in tabs (M3): who answered
// rpc.hello, and the vaults notesd owns.
import type { Hello, Vault } from "../../rpc/gen/api";

export interface WelcomeProps {
  hello: Hello;
  vaults: Vault[];
}

export function Welcome({ hello, vaults }: WelcomeProps) {
  return (
    <main aria-label="Workspace" className="min-w-0 flex-1 overflow-y-auto bg-window">
      <div className="mx-auto max-w-2xl px-8 py-12">
        <h1 className="text-[22px] leading-7 font-semibold">Frostnotes</h1>
        <dl className="mt-6 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-[13px] leading-5">
          <dt className="text-secondary">Server</dt>
          <dd data-testid="hello-server" className="select-text">
            {hello.server}
          </dd>
          <dt className="text-secondary">Protocol</dt>
          <dd data-testid="hello-protocol" className="select-text">
            {hello.protocol}
          </dd>
        </dl>
        <h2 className="mt-10 text-[15px] leading-5 font-semibold">Vaults</h2>
        {vaults.length === 0 ? (
          <p className="mt-3 text-secondary">
            No vaults yet. Register one with <code className="select-text">notesctl vault add PATH</code>.
          </p>
        ) : (
          <ul aria-label="Vault list" className="mt-3 divide-y divide-separator border-y border-separator">
            {vaults.map((v) => (
              <li key={v.id} className="flex items-baseline gap-4 py-2">
                <span className="min-w-0 shrink-0 font-medium">{v.name}</span>
                <span className="min-w-0 flex-1 truncate text-secondary select-text" title={v.path}>
                  {v.path}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </main>
  );
}
