import { useState } from "react";
import PageHeading from "../components/ui/PageHeading";
import LeavePolicyRules from "../components/tier-makeup/LeavePolicyRules";
import LeavePolicyTestPanel from "../components/tier-makeup/LeavePolicyTestPanel";

type Tab = "rules" | "test";

const tabs: { id: Tab; label: string }[] = [
  { id: "rules", label: "Course Rules" },
  { id: "test", label: "Test" },
];

export default function LeavePolicy() {
  const [activeTab, setActiveTab] = useState<Tab>("rules");

  return (
    <div>
      <PageHeading>SAT Verbal Leave Policy</PageHeading>

      {/* Tab navigation */}
      <div className="border-b border-gray-200 mb-4">
        <nav className="flex gap-0" aria-label="Tabs">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px ${
                activeTab === tab.id
                  ? "border-[var(--color-wi-primary)] text-[var(--color-wi-primary)]"
                  : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
              }`}
              aria-current={activeTab === tab.id ? "page" : undefined}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab content */}
      {activeTab === "rules" && <LeavePolicyRules />}
      {activeTab === "test" && <LeavePolicyTestPanel />}
    </div>
  );
}
