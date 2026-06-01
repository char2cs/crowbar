"use client";

import { mergeProps } from "@base-ui/react/merge-props";
import { useRender } from "@base-ui/react/use-render";
import { cva, type VariantProps } from "class-variance-authority";
import { animate, motion, useMotionValue } from "framer-motion";
import { PanelLeftIcon } from "lucide-react";
import * as React from "react";
import { useEffect, useRef, useState } from "react";
import type { Icon as PhosphorIcon } from "@phosphor-icons/react";
import { useMediaQuery } from "@/hooks/use-media-query";
import { cn } from "@/lib/utils";
import { Button, type ButtonProps } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetDescription,
  SheetHeader,
  SheetPopup,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipPopup,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import TooltipCompound from "@/components/ui/tooltip";

const SIDEBAR_COOKIE_NAME: string = "sidebar_state";
const SIDEBAR_COOKIE_MAX_AGE: number = 60 * 60 * 24 * 7;
const SIDEBAR_WIDTH: string = "16rem";
const SIDEBAR_WIDTH_MOBILE: string = "18rem";
const SIDEBAR_WIDTH_ICON: string = "3rem";
const SIDEBAR_KEYBOARD_SHORTCUT: string = "b";

const sidebarMenuButtonVariants = cva(
  "peer/menu-button flex w-full items-center gap-2 overflow-hidden rounded-lg p-2 text-left text-sm outline-hidden ring-sidebar-ring transition-[width,height,padding] hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-2 active:bg-sidebar-accent active:text-sidebar-accent-foreground disabled:pointer-events-none disabled:opacity-50 group-has-data-[sidebar=menu-action]/menu-item:pe-8 aria-disabled:pointer-events-none aria-disabled:opacity-50 data-[active=true]:bg-sidebar-accent data-[active=true]:font-medium data-[active=true]:text-sidebar-accent-foreground data-[state=open]:hover:bg-sidebar-accent data-[state=open]:hover:text-sidebar-accent-foreground group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:p-2! [&>span:last-child]:truncate [&>svg:not([class*='size-'])]:size-4 [&>svg]:shrink-0",
  {
    defaultVariants: {
      size: "default",
      variant: "default",
    },
    variants: {
      size: {
        default: "h-8 text-sm",
        lg: "h-12 text-sm group-data-[collapsible=icon]:p-0!",
        sm: "h-7 text-xs",
      },
      variant: {
        default: "hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
        outline:
          "bg-background shadow-[0_0_0_1px_var(--sidebar-border)] hover:bg-sidebar-accent hover:text-sidebar-accent-foreground hover:shadow-[0_0_0_1px_var(--sidebar-accent)]",
      },
    },
  },
);

export type SidebarContextProps = {
  state: "expanded" | "collapsed";
  open: boolean;
  setOpen: (open: boolean) => void;
  openMobile: boolean;
  setOpenMobile: (open: boolean) => void;
  isMobile: boolean;
  toggleSidebar: () => void;
};

export const SidebarContext: React.Context<SidebarContextProps | null> =
  React.createContext<SidebarContextProps | null>(null);

export function useSidebar(): SidebarContextProps {
  const context = React.useContext(SidebarContext);
  if (!context) {
    throw new Error("useSidebar must be used within a SidebarProvider.");
  }

  return context;
}

export function SidebarProvider({
  defaultOpen = true,
  open: openProp,
  onOpenChange: setOpenProp,
  className,
  style,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}): React.ReactElement {
  const isMobile = useMediaQuery("max-md");
  const [openMobile, setOpenMobile] = React.useState(false);

  // This is the internal state of the sidebar.
  // We use openProp and setOpenProp for control from outside the component.
  const [_open, _setOpen] = React.useState(defaultOpen);
  const open = openProp ?? _open;
  const setOpen = React.useCallback(
    async (value: boolean | ((value: boolean) => boolean)) => {
      const openState = typeof value === "function" ? value(open) : value;
      if (setOpenProp) {
        setOpenProp(openState);
      } else {
        _setOpen(openState);
      }

      // This sets the cookie to keep the sidebar state.
      await cookieStore.set({
        expires: Date.now() + SIDEBAR_COOKIE_MAX_AGE * 1000,
        name: SIDEBAR_COOKIE_NAME,
        path: "/",
        value: String(openState),
      });
    },
    [setOpenProp, open],
  );

  // Helper to toggle the sidebar.
  const toggleSidebar = React.useCallback(() => {
    return isMobile ? setOpenMobile((open) => !open) : setOpen((open) => !open);
  }, [isMobile, setOpen]);

  // Adds a keyboard shortcut to toggle the sidebar.
  React.useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent): void => {
      if (
        event.key === SIDEBAR_KEYBOARD_SHORTCUT &&
        (event.metaKey || event.ctrlKey)
      ) {
        event.preventDefault();
        toggleSidebar();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [toggleSidebar]);

  // We add a state so that we can do data-state="expanded" or "collapsed".
  // This makes it easier to style the sidebar with Tailwind classes.
  const state = open ? "expanded" : "collapsed";

  const contextValue = React.useMemo<SidebarContextProps>(
    () => ({
      isMobile,
      open,
      openMobile,
      setOpen,
      setOpenMobile,
      state,
      toggleSidebar,
    }),
    [state, open, setOpen, isMobile, openMobile, toggleSidebar],
  );

  return (
    <SidebarContext.Provider value={contextValue}>
      <div
        className={cn(
          "group/sidebar-wrapper flex min-h-svh w-full has-data-[variant=inset]:bg-sidebar",
          className,
        )}
        data-slot="sidebar-wrapper"
        style={
          {
            "--sidebar-width": SIDEBAR_WIDTH,
            "--sidebar-width-icon": SIDEBAR_WIDTH_ICON,
            ...style,
          } as React.CSSProperties
        }
        {...props}
      >
        {children}
      </div>
    </SidebarContext.Provider>
  );
}

export function Sidebar({
  side = "left",
  variant = "sidebar",
  collapsible = "offcanvas",
  className,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  side?: "left" | "right";
  variant?: "sidebar" | "floating" | "inset";
  collapsible?: "offcanvas" | "icon" | "none";
}): React.ReactElement {
  const { isMobile, state, openMobile, setOpenMobile } = useSidebar();

  if (collapsible === "none") {
    return (
      <div
        className={cn(
          "flex h-full w-(--sidebar-width) flex-col bg-sidebar text-sidebar-foreground",
          className,
        )}
        data-slot="sidebar"
        {...props}
      >
        {children}
      </div>
    );
  }

  if (isMobile) {
    return (
      <Sheet onOpenChange={setOpenMobile} open={openMobile} {...props}>
        <SheetPopup
          className="w-(--sidebar-width) bg-sidebar p-0 text-sidebar-foreground [&>button]:hidden"
          data-mobile="true"
          data-sidebar="sidebar"
          data-slot="sidebar"
          side={side}
          style={
            {
              "--sidebar-width": SIDEBAR_WIDTH_MOBILE,
            } as React.CSSProperties
          }
        >
          <SheetHeader className="sr-only">
            <SheetTitle>Sidebar</SheetTitle>
            <SheetDescription>Displays the mobile sidebar.</SheetDescription>
          </SheetHeader>
          <div className="flex h-full w-full flex-col">{children}</div>
        </SheetPopup>
      </Sheet>
    );
  }

  return (
    <div
      className="group peer hidden text-sidebar-foreground md:block"
      data-collapsible={state === "collapsed" ? collapsible : ""}
      data-side={side}
      data-slot="sidebar"
      data-state={state}
      data-variant={variant}
    >
      {/* This is what handles the sidebar gap on desktop */}
      <div
        className={cn(
          "relative w-(--sidebar-width) bg-transparent transition-[width] duration-200 ease-linear",
          "group-data-[collapsible=offcanvas]:w-0",
          "group-data-[side=right]:rotate-180",
          variant === "floating" || variant === "inset"
            ? "group-data-[collapsible=icon]:w-[calc(var(--sidebar-width-icon)+(--spacing(4)))]"
            : "group-data-[collapsible=icon]:w-(--sidebar-width-icon)",
        )}
        data-slot="sidebar-gap"
      />
      <div
        className={cn(
          "fixed inset-y-0 z-10 hidden h-svh w-(--sidebar-width) transition-[left,right,width] duration-200 ease-linear md:flex",
          side === "left"
            ? "left-0 group-data-[collapsible=offcanvas]:left-[calc(var(--sidebar-width)*-1)]"
            : "right-0 group-data-[collapsible=offcanvas]:right-[calc(var(--sidebar-width)*-1)]",
          // Adjust the padding for floating and inset variants.
          variant === "floating" || variant === "inset"
            ? "p-2 group-data-[collapsible=icon]:w-[calc(var(--sidebar-width-icon)+(--spacing(4))+2px)]"
            : "group-data-[collapsible=icon]:w-(--sidebar-width-icon) group-data-[side=left]:border-r group-data-[side=right]:border-l",
          className,
        )}
        data-slot="sidebar-container"
        {...props}
      >
        <div
          className="flex h-full w-full flex-col bg-sidebar group-data-[variant=floating]:rounded-lg group-data-[variant=floating]:border group-data-[variant=floating]:border-sidebar-border group-data-[variant=floating]:shadow-sm/5"
          data-sidebar="sidebar"
          data-slot="sidebar-inner"
        >
          {children}
        </div>
      </div>
    </div>
  );
}

export function SidebarTrigger({
  className,
  onClick,
  ...props
}: React.ComponentProps<typeof Button>): React.ReactElement {
  const { toggleSidebar } = useSidebar();

  return (
    <Button
      className={cn("size-7", className)}
      data-sidebar="trigger"
      data-slot="sidebar-trigger"
      onClick={(event: React.MouseEvent<HTMLButtonElement>) => {
        onClick?.(event);
        toggleSidebar();
      }}
      size="icon"
      variant="ghost"
      {...props}
    >
      <PanelLeftIcon />
      <span className="sr-only">Toggle Sidebar</span>
    </Button>
  );
}

export function SidebarRail({
  className,
  ...props
}: React.ComponentProps<"button">): React.ReactElement {
  const { toggleSidebar } = useSidebar();

  return (
    <button
      aria-label="Toggle Sidebar"
      className={cn(
        "absolute inset-y-0 z-20 hidden w-4 -translate-x-1/2 transition-all ease-linear after:absolute after:inset-y-0 after:left-1/2 after:w-[2px] hover:after:bg-sidebar-border group-data-[side=left]:-right-4 group-data-[side=right]:left-0 sm:flex",
        "in-data-[side=left]:cursor-w-resize in-data-[side=right]:cursor-e-resize",
        "[[data-side=left][data-state=collapsed]_&]:cursor-e-resize [[data-side=right][data-state=collapsed]_&]:cursor-w-resize",
        "group-data-[collapsible=offcanvas]:translate-x-0 hover:group-data-[collapsible=offcanvas]:bg-sidebar group-data-[collapsible=offcanvas]:after:left-full",
        "[[data-side=left][data-collapsible=offcanvas]_&]:-right-2",
        "[[data-side=right][data-collapsible=offcanvas]_&]:-left-2",
        className,
      )}
      data-sidebar="rail"
      data-slot="sidebar-rail"
      onClick={toggleSidebar}
      tabIndex={-1}
      title="Toggle Sidebar"
      type="button"
      {...props}
    />
  );
}

export function SidebarInset({
  className,
  ...props
}: React.ComponentProps<"main">): React.ReactElement {
  return (
    <main
      className={cn(
        "relative flex w-full flex-1 flex-col bg-background",
        "md:peer-data-[variant=inset]:peer-data-[state=collapsed]:ms-2 md:peer-data-[variant=inset]:m-2 md:peer-data-[variant=inset]:ms-0 md:peer-data-[variant=inset]:rounded-xl md:peer-data-[variant=inset]:shadow-sm/5",
        className,
      )}
      data-slot="sidebar-inset"
      {...props}
    />
  );
}

export function SidebarInput({
  className,
  ...props
}: React.ComponentProps<typeof Input>): React.ReactElement {
  return (
    <Input
      className={cn("h-8 w-full bg-background shadow-none", className)}
      data-sidebar="input"
      data-slot="sidebar-input"
      {...props}
    />
  );
}

export function SidebarHeader({
  className,
  ...props
}: React.ComponentProps<"div">): React.ReactElement {
  return (
    <div
      className={cn("flex flex-col gap-2 p-2", className)}
      data-sidebar="header"
      data-slot="sidebar-header"
      {...props}
    />
  );
}

export function SidebarFooter({
  className,
  surface: _surface,
  ...props
}: React.ComponentProps<"div"> & {
  /** Crowbar compat: surface elevation flag (no visual effect) */
  surface?: boolean;
}): React.ReactElement {
  return (
    <div
      className={cn("flex flex-col gap-2 p-2", className)}
      data-sidebar="footer"
      data-slot="sidebar-footer"
      {...props}
    />
  );
}

export function SidebarSeparator({
  className,
  ...props
}: React.ComponentProps<typeof Separator>): React.ReactElement {
  return (
    <Separator
      className={cn("mx-2 w-auto bg-sidebar-border", className)}
      data-sidebar="separator"
      data-slot="sidebar-separator"
      {...props}
    />
  );
}

export function SidebarContent({
  className,
  ...props
}: React.ComponentProps<"div">): React.ReactElement {
  return (
    <ScrollArea
      className="**:data-[slot=scroll-area-scrollbar]:hidden"
      scrollFade
    >
      <div
        className={cn(
          "flex min-h-0 flex-1 flex-col gap-2 overflow-auto group-data-[collapsible=icon]:overflow-hidden",
          className,
        )}
        data-sidebar="content"
        data-slot="sidebar-content"
        {...props}
      />
    </ScrollArea>
  );
}

export function SidebarGroup({
  className,
  ...props
}: React.ComponentProps<"div">): React.ReactElement {
  return (
    <div
      className={cn("relative flex w-full min-w-0 flex-col p-2", className)}
      data-sidebar="group"
      data-slot="sidebar-group"
      {...props}
    />
  );
}

export function SidebarGroupLabel({
  className,
  render,
  ...props
}: useRender.ComponentProps<"div">): React.ReactElement {
  const defaultProps = {
    className: cn(
      "flex h-8 shrink-0 items-center rounded-lg px-2 font-medium text-sidebar-foreground text-xs outline-hidden ring-sidebar-ring transition-[margin,opacity] duration-200 ease-linear focus-visible:ring-2 [&>svg]:size-4 [&>svg]:shrink-0",
      "group-data-[collapsible=icon]:-mt-8 group-data-[collapsible=icon]:opacity-0",
      className,
    ),
    "data-sidebar": "group-label",
    "data-slot": "sidebar-group-label",
  };

  return useRender({
    defaultTagName: "div",
    props: mergeProps(defaultProps, props),
    render,
  });
}

export function SidebarGroupAction({
  className,
  render,
  ...props
}: useRender.ComponentProps<"button">): React.ReactElement {
  const defaultProps = {
    className: cn(
      "absolute top-3.5 right-3 flex aspect-square w-5 items-center justify-center rounded-lg p-0 text-sidebar-foreground outline-hidden ring-sidebar-ring transition-transform hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-2 [&>svg:not([class*='size-'])]:size-4 [&>svg]:shrink-0",
      // Increases the hit area of the button on mobile.
      "after:absolute after:-inset-2 md:after:hidden",
      "group-data-[collapsible=icon]:hidden",
      className,
    ),
    "data-sidebar": "group-action",
    "data-slot": "sidebar-group-action",
  };

  return useRender({
    defaultTagName: "button",
    props: mergeProps(defaultProps, props),
    render,
  });
}

export function SidebarGroupContent({
  className,
  ...props
}: React.ComponentProps<"div">): React.ReactElement {
  return (
    <div
      className={cn("w-full text-sm", className)}
      data-sidebar="group-content"
      data-slot="sidebar-group-content"
      {...props}
    />
  );
}

export function SidebarMenu({
  className,
  ...props
}: React.ComponentProps<"ul">): React.ReactElement {
  return (
    <ul
      className={cn("flex w-full min-w-0 flex-col gap-1", className)}
      data-sidebar="menu"
      data-slot="sidebar-menu"
      {...props}
    />
  );
}

export function SidebarMenuItem({
  className,
  ...props
}: React.ComponentProps<"li">): React.ReactElement {
  return (
    <li
      className={cn("group/menu-item relative", className)}
      data-sidebar="menu-item"
      data-slot="sidebar-menu-item"
      {...props}
    />
  );
}

export function SidebarMenuButton({
  isActive = false,
  variant = "default",
  size = "default",
  tooltip,
  className,
  render,
  ...props
}: useRender.ComponentProps<"button"> & {
  isActive?: boolean;
  tooltip?: string | React.ComponentProps<typeof TooltipPopup>;
} & VariantProps<typeof sidebarMenuButtonVariants>): React.ReactElement {
  const { isMobile, state } = useSidebar();

  const defaultProps = {
    className: cn(sidebarMenuButtonVariants({ size, variant }), className),
    "data-active": isActive,
    "data-sidebar": "menu-button",
    "data-size": size,
    "data-slot": "sidebar-menu-button",
  };

  const buttonProps = mergeProps<"button">(defaultProps, props);

  const buttonElement = useRender({
    defaultTagName: "button",
    props: buttonProps,
    render,
  });

  if (!tooltip) {
    return buttonElement;
  }

  if (typeof tooltip === "string") {
    tooltip = {
      children: tooltip,
    };
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {buttonElement as React.ReactElement}
      </TooltipTrigger>
      <TooltipPopup
        align="center"
        hidden={state !== "collapsed" || isMobile}
        side="right"
        {...tooltip}
      />
    </Tooltip>
  );
}

export function SidebarMenuAction({
  className,
  showOnHover = false,
  render,
  ...props
}: useRender.ComponentProps<"button"> & {
  showOnHover?: boolean;
}): React.ReactElement {
  const defaultProps = {
    className: cn(
      "absolute top-1.5 right-1 flex aspect-square w-5 items-center justify-center rounded-lg p-0 text-sidebar-foreground outline-hidden ring-sidebar-ring transition-transform hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-2 peer-hover/menu-button:text-sidebar-accent-foreground [&>svg:not([class*='size-'])]:size-4 [&>svg]:shrink-0",
      // Increases the hit area of the button on mobile.
      "after:absolute after:-inset-2 md:after:hidden",
      "peer-data-[size=sm]/menu-button:top-1",
      "peer-data-[size=default]/menu-button:top-1.5",
      "peer-data-[size=lg]/menu-button:top-2.5",
      "group-data-[collapsible=icon]:hidden",
      showOnHover &&
        "group-focus-within/menu-item:opacity-100 group-hover/menu-item:opacity-100 data-[state=open]:opacity-100 peer-data-[active=true]/menu-button:text-sidebar-accent-foreground md:opacity-0",
      className,
    ),
    "data-sidebar": "menu-action",
    "data-slot": "sidebar-menu-action",
  };

  return useRender({
    defaultTagName: "button",
    props: mergeProps<"button">(defaultProps, props),
    render,
  });
}

export function SidebarMenuBadge({
  className,
  ...props
}: React.ComponentProps<"div">): React.ReactElement {
  return (
    <div
      className={cn(
        "pointer-events-none absolute right-1 flex h-5 min-w-5 select-none items-center justify-center rounded-lg px-1 font-medium text-sidebar-foreground text-xs tabular-nums",
        "peer-hover/menu-button:text-sidebar-accent-foreground peer-data-[active=true]/menu-button:text-sidebar-accent-foreground",
        "peer-data-[size=sm]/menu-button:top-1",
        "peer-data-[size=default]/menu-button:top-1.5",
        "peer-data-[size=lg]/menu-button:top-2.5",
        "group-data-[collapsible=icon]:hidden",
        className,
      )}
      data-sidebar="menu-badge"
      data-slot="sidebar-menu-badge"
      {...props}
    />
  );
}

export function SidebarMenuSkeleton({
  className,
  showIcon = false,
  ...props
}: React.ComponentProps<"div"> & {
  showIcon?: boolean;
}): React.ReactElement {
  // Random width between 50 to 90%.
  const width = React.useMemo(() => {
    return `${Math.floor(Math.random() * 40) + 50}%`;
  }, []);

  return (
    <div
      className={cn("flex h-8 items-center gap-2 rounded-lg px-2", className)}
      data-sidebar="menu-skeleton"
      data-slot="sidebar-menu-skeleton"
      {...props}
    >
      {showIcon && (
        <Skeleton
          className="size-4 rounded-lg"
          data-sidebar="menu-skeleton-icon"
        />
      )}
      <Skeleton
        className="h-4 max-w-(--skeleton-width) flex-1"
        data-sidebar="menu-skeleton-text"
        style={
          {
            "--skeleton-width": width,
          } as React.CSSProperties
        }
      />
    </div>
  );
}

export function SidebarMenuSub({
  className,
  ...props
}: React.ComponentProps<"ul">): React.ReactElement {
  return (
    <ul
      className={cn(
        "mx-3.5 flex min-w-0 translate-x-px flex-col gap-1 border-sidebar-border border-l px-2.5 py-0.5",
        "group-data-[collapsible=icon]:hidden",
        className,
      )}
      data-sidebar="menu-sub"
      data-slot="sidebar-menu-sub"
      {...props}
    />
  );
}

export function SidebarMenuSubItem({
  className,
  ...props
}: React.ComponentProps<"li">): React.ReactElement {
  return (
    <li
      className={cn("group/menu-sub-item relative", className)}
      data-sidebar="menu-sub-item"
      data-slot="sidebar-menu-sub-item"
      {...props}
    />
  );
}

export function SidebarMenuSubButton({
  size = "md",
  isActive = false,
  className,
  render,
  ...props
}: useRender.ComponentProps<"a"> & {
  size?: "sm" | "md";
  isActive?: boolean;
}): React.ReactElement {
  const defaultProps = {
    className: cn(
      "flex h-8 min-w-0 -translate-x-px items-center gap-2 overflow-hidden rounded-lg px-2 text-sidebar-foreground outline-hidden ring-sidebar-ring hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-2 active:bg-sidebar-accent active:text-sidebar-accent-foreground disabled:pointer-events-none disabled:opacity-50 aria-disabled:pointer-events-none aria-disabled:opacity-50 sm:h-7 [&>span:last-child]:truncate [&>svg:not([class*='size-'])]:size-4 [&>svg]:shrink-0 [&>svg]:text-sidebar-accent-foreground",
      "data-[active=true]:bg-sidebar-accent data-[active=true]:text-sidebar-accent-foreground",
      size === "sm" && "text-xs",
      size === "md" && "text-sm",
      "group-data-[collapsible=icon]:hidden",
      className,
    ),
    "data-active": isActive,
    "data-sidebar": "menu-sub-button",
    "data-size": size,
    "data-slot": "sidebar-menu-sub-button",
  };

  return useRender({
    defaultTagName: "a",
    props: mergeProps<"a">(defaultProps, props),
    render,
  });
}

// ─── Crowbar-specific sidebar extensions ────────────────────────────────────

export function SidebarPanel({
  children,
  className,
  framed = false,
  ...props
}: React.ComponentProps<"div"> & {
  children: React.ReactNode;
  framed?: boolean;
}): React.ReactElement {
  return (
    <div
      className={cn(
        "flex h-full min-h-0 flex-col",
        framed && "rounded-lg border border-border/70",
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

export function SidebarComposerBody({
  children,
  className,
  ...props
}: React.ComponentProps<"div"> & {
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <div
      className={cn(
        "overflow-hidden rounded-xl border border-border/60 bg-background",
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

export const SidebarHeaderSearch = React.forwardRef<
  HTMLInputElement,
  Omit<React.ComponentProps<typeof Input>, "onChange" | "value" | "size"> & {
    value: string;
    onChange: (value: string) => void;
    leftIcon: PhosphorIcon;
  }
>(function SidebarHeaderSearch(
  { value, onChange, leftIcon: LeftIcon, placeholder = "Search", className, ...props },
  ref,
) {
  return (
    <span className={cn("relative inline-flex min-w-0 flex-1", className)}>
      {LeftIcon && (
        <span className="pointer-events-none absolute inset-y-0 start-2 flex items-center text-muted-foreground/72">
          <LeftIcon className="size-3.5" />
        </span>
      )}
      <Input
        nativeInput
        ref={ref}
        value={value}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
        placeholder={placeholder}
        size="sm"
        unstyled
        className="h-6 min-w-0 w-full rounded-md px-2 ps-7 text-sm bg-transparent border-transparent outline-none"
        {...props}
      />
    </span>
  );
});

export const SidebarHeaderIconButton = React.forwardRef<
  HTMLButtonElement,
  Omit<ButtonProps, "variant">
>(function SidebarHeaderIconButton({ className, ...props }, ref) {
  return (
    <Button
      ref={ref}
      type="button"
      variant="ghost"
      compact
      className={cn("size-6 rounded-md p-0", className)}
      {...props}
    />
  );
});

export function SidebarEmptyState({
  children,
  className,
  ...props
}: React.ComponentProps<"div"> & {
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <div
      className={cn(
        "ui-font ui-text-sm flex min-h-24 select-none items-center justify-center px-3 py-6 text-center text-muted-foreground",
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

export function SidebarEmptyActionState({
  icon,
  message,
  description,
  actionLabel,
  onAction,
  actionDisabled = false,
  className,
  actionClassName,
  tone = "neutral",
  children,
  ...props
}: React.ComponentProps<"div"> & {
  icon?: React.ReactNode;
  message: React.ReactNode;
  description?: React.ReactNode;
  actionLabel?: React.ReactNode;
  onAction?: () => void;
  actionDisabled?: boolean;
  actionClassName?: string;
  tone?: "neutral" | "error" | "success";
}): React.ReactElement {
  return (
    <div
      className={cn(
        "ui-font flex min-h-24 select-none flex-col items-center justify-center gap-1.5 px-3 py-6 text-center text-muted-foreground",
        className,
      )}
      {...props}
    >
      {icon ? (
        <span
          className={cn(
            "mb-0.5 flex size-7 items-center justify-center text-muted-foreground",
            tone === "error" && "text-destructive",
            tone === "success" && "text-success",
          )}
        >
          {icon}
        </span>
      ) : null}
      <div
        className={cn(
          "ui-text-sm leading-[1.35]",
          tone === "error" && "text-destructive",
          tone === "success" && "text-success",
        )}
      >
        {message}
      </div>
      {description ? (
        <div className="ui-text-xs max-w-[24ch] leading-[1.35] text-muted-foreground">
          {description}
        </div>
      ) : null}
      {actionLabel && onAction ? (
        <Button
          type="button"
          variant="ghost"
          compact
          className={cn("ui-text-xs h-6 px-2 text-muted-foreground hover:text-foreground", actionClassName)}
          disabled={actionDisabled}
          onClick={onAction}
        >
          {actionLabel}
        </Button>
      ) : null}
      {children}
    </div>
  );
}

export function SidebarListItem({
  children,
  active = false,
  leading,
  trailing,
  className,
  contentClassName,
  ...props
}: React.ComponentProps<"button"> & {
  children: React.ReactNode;
  active?: boolean;
  leading?: React.ReactNode;
  trailing?: React.ReactNode;
  contentClassName?: string;
}): React.ReactElement {
  return (
    <button
      type="button"
      className={cn(
        "ui-font flex min-h-7 w-full min-w-0 cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left text-muted-foreground transition-[background-color,color]",
        "hover:bg-accent/70 hover:text-foreground focus-visible:bg-accent/70 focus-visible:text-foreground focus-visible:outline-none",
        active && "bg-accent/80 text-foreground",
        className,
      )}
      {...props}
    >
      {leading ? (
        <span className="flex shrink-0 items-center justify-center">{leading}</span>
      ) : null}
      <span className={cn("min-w-0 flex-1", contentClassName)}>{children}</span>
      {trailing ? <span className="shrink-0 text-muted-foreground">{trailing}</span> : null}
    </button>
  );
}

const SIDEBAR_SECTION_PAGER_SPRING = {
  type: "spring" as const,
  stiffness: 360,
  damping: 36,
  mass: 0.8,
};

export interface SidebarSectionPagerItem {
  id: string;
  content: React.ReactNode;
  disabled?: boolean;
}

export function SidebarSectionPager({
  items,
  value,
  className,
}: {
  items: SidebarSectionPagerItem[];
  value: string;
  onChange: (value: string) => void;
  className?: string;
}): React.ReactElement {
  const viewportRef = useRef<HTMLDivElement>(null);
  const animationStopRef = useRef<(() => void) | null>(null);
  const x = useMotionValue(0);
  const [pagerWidth, setPagerWidth] = useState(0);
  const activeIndex = Math.max(
    0,
    items.findIndex((item) => item.id === value),
  );
  const activeX = pagerWidth > 0 ? -activeIndex * pagerWidth : 0;

  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;

    const updateWidth = () => setPagerWidth(viewport.clientWidth);
    updateWidth();

    const resizeObserver = new ResizeObserver(updateWidth);
    resizeObserver.observe(viewport);

    return () => resizeObserver.disconnect();
  }, []);

  useEffect(() => {
    if (pagerWidth <= 0) return;

    animationStopRef.current?.();
    const controls = animate(x, activeX, SIDEBAR_SECTION_PAGER_SPRING);
    animationStopRef.current = () => controls.stop();
    return () => controls.stop();
  }, [activeX, pagerWidth, x]);

  useEffect(() => {
    return () => {
      animationStopRef.current?.();
    };
  }, []);

  return (
    <div ref={viewportRef} className={cn("min-h-0 overflow-hidden", className)}>
      <motion.div className="flex h-full min-h-0" style={{ x }}>
        {items.map((item) => {
          const isActive = item.id === value;
          return (
            <div
              key={item.id}
              aria-hidden={!isActive}
              className={cn(
                "h-full min-w-full cursor-auto overflow-hidden",
                !isActive && "pointer-events-none",
              )}
            >
              {item.content}
            </div>
          );
        })}
      </motion.div>
    </div>
  );
}

export interface SidebarSectionSwitcherItem {
  id: string;
  label: string;
  icon?: React.ReactNode;
  disabled?: boolean;
}

export function SidebarSectionSwitcher({
  items,
  value,
  onChange,
  className,
  itemClassName,
}: {
  items: SidebarSectionSwitcherItem[];
  value: string;
  onChange: (value: string) => void;
  className?: string;
  itemClassName?: string;
}): React.ReactElement {
  return (
    <div
      role="tablist"
      className={cn(
        "mx-auto flex h-7 w-fit max-w-full shrink-0 select-none items-center justify-center gap-1 rounded-full bg-muted/45 p-0.5",
        className,
      )}
    >
      {items.map((item) => {
        const selected = item.id === value;
        const button = (
          <button
            key={item.id}
            type="button"
            role="tab"
            aria-selected={selected}
            aria-label={item.label}
            disabled={item.disabled}
            className={cn(
              "ui-font ui-text-xs flex h-6 min-w-0 items-center justify-center gap-1.5 rounded-full outline-none transition-[background-color,color,width,padding]",
              selected
                ? "max-w-28 bg-accent px-2 text-accent-foreground"
                : "w-7 px-0 text-muted-foreground hover:bg-accent/70 hover:text-accent-foreground",
              item.disabled && "cursor-not-allowed opacity-50",
              itemClassName,
            )}
            onClick={() => onChange(item.id)}
          >
            {item.icon ? (
              <span className="flex size-4 shrink-0 items-center justify-center">{item.icon}</span>
            ) : null}
            {selected ? (
              <span className="min-w-0 truncate whitespace-nowrap">{item.label}</span>
            ) : null}
          </button>
        );

        return (
          <TooltipCompound key={item.id} content={item.label} side="bottom">
            {button}
          </TooltipCompound>
        );
      })}
    </div>
  );
}
