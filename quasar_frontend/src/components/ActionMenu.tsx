import type { ReactNode } from "react";
import { DropdownMenu } from "./DropdownMenu";

export type ActionMenuItem = {
  id: string;
  label: string;
  onClick: () => void;
  danger?: boolean;
  disabled?: boolean;
};

export function ActionMenu({
  items,
  title = "Opções",
  align = "end",
  buttonLabel,
  icon,
}: {
  items: ActionMenuItem[];
  title?: string;
  align?: "start" | "end";
  buttonLabel?: string;
  icon?: ReactNode;
}) {
  return (
    <DropdownMenu
      align={align}
      className="action-menu"
      trigger={({ toggle, open }) => (
        <button
          type="button"
          className={buttonLabel && !icon ? "btn" : "btn btn--icon btn--icon-menu"}
          title={title}
          aria-label={title}
          aria-haspopup="menu"
          aria-expanded={open}
          onClick={toggle}
        >
          {icon ?? buttonLabel ?? "⋮"}
        </button>
      )}
    >
      {({ close }) =>
        items.map((it) => (
          <button
            key={it.id}
            type="button"
            className={`action-menu__item ${it.danger ? "action-menu__item--danger" : ""}`}
            disabled={it.disabled}
            onClick={() => {
              close();
              it.onClick();
            }}
          >
            {it.label}
          </button>
        ))
      }
    </DropdownMenu>
  );
}
