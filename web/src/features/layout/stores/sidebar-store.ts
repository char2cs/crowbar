import { create } from "zustand";
import { createSelectors } from "@/utils/zustand-selectors";

interface SidebarState {
  activePath?: string;
  updateActivePath: (path: string) => void;
  sidebarVisible: boolean;
  setSidebarVisible: (visible: boolean) => void;
}

const useSidebarStoreBase = create<SidebarState>()((set) => ({
  activePath: undefined,
  updateActivePath: (path: string) => {
    set({ activePath: path });
  },
  sidebarVisible: true,
  setSidebarVisible: (visible: boolean) => {
    set({ sidebarVisible: visible });
  },
}));

export const useSidebarStore = createSelectors(useSidebarStoreBase);
