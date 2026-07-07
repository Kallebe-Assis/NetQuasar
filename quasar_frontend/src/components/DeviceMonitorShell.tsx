import type { LucideIcon } from "lucide-react";
import "../styles/mikrotik-noc.css";

export type MonitorTab<T extends string> = {
  id: T;
  label: string;
  icon?: LucideIcon;
};

type Props<T extends string> = {
  tabs: MonitorTab<T>[];
  activeTab: T;
  onTab: (tab: T) => void;
  title: string;
  subtitle?: string;
  online?: boolean;
  meta?: React.ReactNode;
  toolbar?: React.ReactNode;
  children: React.ReactNode;
};

export function DeviceMonitorShell<T extends string>({
  tabs,
  activeTab,
  onTab,
  title,
  subtitle,
  online = true,
  meta,
  toolbar,
  children,
}: Props<T>) {
  return (
    <div className="mk-noc">
      <div className="mk-noc-body">
        <header className="mk-noc-header">
          <div>
            <h2>
              {title}
              <span className={`mk-noc-badge ${online ? "mk-noc-badge--on" : "mk-noc-badge--off"}`}>
                {online ? "ONLINE" : "OFFLINE"}
              </span>
            </h2>
            {subtitle ? <p className="mk-noc-subtitle">{subtitle}</p> : null}
            {meta ? <div className="mk-noc-meta">{meta}</div> : null}
          </div>
          {toolbar ? <div className="mk-noc-toolbar">{toolbar}</div> : null}
        </header>

        <nav className="mk-noc-tabs" aria-label="Secções do monitor">
          {tabs.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              type="button"
              className={`mk-noc-tab ${activeTab === id ? "mk-noc-tab--active" : ""}`}
              onClick={() => onTab(id)}
            >
              {Icon ? <Icon size={13} /> : null}
              {label}
            </button>
          ))}
        </nav>

        <div className="mk-noc-content">{children}</div>
      </div>
    </div>
  );
}
