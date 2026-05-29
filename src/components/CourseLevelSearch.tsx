import { useState, useEffect } from "react";
import { Search } from "lucide-react";

type CourseLevelSearchProps = {
  value: string;
  onChange: (value: string) => void;
};

export default function CourseLevelSearch({ value, onChange }: CourseLevelSearchProps) {
  const [internalValue, setInternalValue] = useState(value);

  useEffect(() => {
    setInternalValue(value);
  }, [value]);

  useEffect(() => {
    const timeout = setTimeout(() => {
      onChange(internalValue);
    }, 300);
    return () => clearTimeout(timeout);
  }, [internalValue]);

  const handleClear = () => {
    setInternalValue("");
    onChange("");
  };

  return (
    <div className="relative">
      <div className="absolute inset-y-0 left-0 pl-2.5 flex items-center pointer-events-none">
        <Search className="h-4 w-4 text-gray-400" />
      </div>
      <input
        type="text"
        value={internalValue}
        onChange={(e) => setInternalValue(e.target.value)}
        placeholder="Search courses, subjects, or groups..."
        className="block w-full pl-8 pr-8 py-1.5 text-sm border border-gray-300 rounded-sm bg-white placeholder-gray-400 focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500"
      />
      {internalValue && (
        <button
          onClick={handleClear}
          className="absolute inset-y-0 right-0 pr-2.5 flex items-center text-gray-400 hover:text-gray-600"
        >
          ×
        </button>
      )}
    </div>
  );
}
