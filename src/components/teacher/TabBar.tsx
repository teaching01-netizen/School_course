type Tab<T extends string> = { id: T; label: string };

type TabBarProps<T extends string> = {
  tabs: readonly Tab<T>[];
  activeTab: T;
  onChange: (tabId: T) => void;
};

export default function TabBar<T extends string>({ tabs, activeTab, onChange }: TabBarProps<T>) {
  return (
    <div className="mb-4 flex rounded-sm border border-gray-300 bg-white text-sm">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => onChange(tab.id)}
          className={`flex items-center gap-1 px-4 py-1.5 ${
            activeTab === tab.id
              ? 'bg-gray-100 text-gray-900 font-medium'
              : 'text-gray-500 hover:text-gray-900'
          }`}
        >
          {tab.label}
        </button>
      ))}
    </div>
  );
}
