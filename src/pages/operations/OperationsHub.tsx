import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import PageHeading from "../../components/ui/PageHeading";
import { SitInRulesSection } from "./SitInRulesSection";
import { ActiveCoursesSection } from "./ActiveCoursesSection";
import { FormSettingsSection } from "./FormSettingsSection";
import { StaffAbsenceRulesSection } from "./StaffAbsenceRulesSection";
import SitInRuleInventoryPage from "./SitInRuleInventoryPage";

type SectionTab = "sit-in-rules" | "form-settings" | "staff-absence-rules" | "active-courses" | "rule-inventory";

const TABS: { key: SectionTab; label: string }[] = [
  { key: "sit-in-rules", label: "Sit-in Rules" },
  { key: "form-settings", label: "Form Settings" },
  { key: "staff-absence-rules", label: "Staff Absence Rules" },
  { key: "active-courses", label: "Active Courses" },
  { key: "rule-inventory", label: "Rule Inventory" },
];

export default function OperationsHub() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tabFromUrl = searchParams.get("tab") as SectionTab | null;
  const initialTab = tabFromUrl && TABS.some((t) => t.key === tabFromUrl) ? tabFromUrl : "sit-in-rules";
  const [activeTab, setActiveTab] = useState<SectionTab>(initialTab);

  function handleTabChange(key: SectionTab) {
    setActiveTab(key);
    setSearchParams(key === "sit-in-rules" ? {} : { tab: key }, { replace: true });
  }

  return (
    <div className="w-full">
      <div className="mb-5">
        <div className="flex items-center gap-3">
          <PageHeading>Operations Configuration</PageHeading>
          <Link
            to="/course-levels"
            className="text-xs font-medium text-[var(--color-wi-primary)] hover:underline"
          >
            Course Levels
          </Link>
        </div>
        <p className="text-sm text-gray-500">Manage sit-in rules, active course mappings, form settings, and operational policies.</p>
      </div>

      <div className="flex gap-0 border-b border-gray-200 overflow-x-auto scrollbar-hide">
        {TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => handleTabChange(tab.key)}
            className={`shrink-0 px-4 py-2.5 text-sm font-medium transition-colors border-b-2 -mb-px ${
              activeTab === tab.key
                ? "border-[var(--color-wi-primary)] text-[var(--color-wi-primary)]"
                : "border-transparent text-gray-500 hover:text-gray-700"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      <div className="mt-5">
        {activeTab === "sit-in-rules" ? <SitInRulesSection /> : null}
        {activeTab === "form-settings" ? <FormSettingsSection /> : null}
        {activeTab === "staff-absence-rules" ? <StaffAbsenceRulesSection /> : null}
        {activeTab === "active-courses" ? <ActiveCoursesSection /> : null}
        {activeTab === "rule-inventory" ? <SitInRuleInventoryPage /> : null}
      </div>
    </div>
  );
}
