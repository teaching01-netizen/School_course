import{c as v,j as r,f,b as y}from"./PreflightIndicator-CXNy5hSU.js";import"./iframe-icl5DO9y.js";import"./preload-helper-PPVm8Dsz.js";function N(e){const o=e.get("kind"),a=e.get("course_id"),t=e.get("teacher_id"),n=e.get("start_at"),s=e.get("end_at");if(!o||!a||!t||!n||!s)return null;const i=e.get("student_count");return{details:{kind:o,requested:{course_id:a,teacher_id:t,room_id:e.get("room_id"),start_at:n,end_at:s},conflicts:[],student_count:i&&!Number.isNaN(Number(i))?Number(i):void 0},teacherName:e.get("teacher")??void 0,roomName:e.get("room")??void 0,studentName:e.get("student")??void 0,studentId:e.get("student_id")??void 0}}function g({context:e,coursesById:o,onDismiss:a}){const{details:t,teacherName:n,roomName:s}=e,i=n&&t.requested.teacher_id?new Map([[t.requested.teacher_id,{id:t.requested.teacher_id,username:n,role:"Teacher"}]]):void 0,x=s&&t.requested.room_id?new Map([[t.requested.room_id,{id:t.requested.room_id,name:s,capacity:null}]]):void 0,_=v(t,{coursesById:o,teachersById:i,roomsById:x});return r.jsx("div",{className:"rounded-md border border-red-200 bg-[var(--color-wi-danger-bg)] px-3 py-2.5 text-[13px]",role:"alert","aria-label":"Conflict you are finding alternatives for",children:r.jsxs("div",{className:"flex items-start gap-2",children:[r.jsx("span",{"aria-hidden":"true",className:"mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-red)] text-[10px] font-bold text-white",children:"✕"}),r.jsxs("div",{className:"min-w-0 flex-1",children:[r.jsx("p",{className:"text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]",children:"Finding alternatives for"}),r.jsx("p",{className:"mt-0.5 font-medium text-[var(--color-wi-red)]",children:_}),r.jsxs("p",{className:"mt-0.5 text-xs text-[var(--color-wi-text-light)]",children:["Requested · ",r.jsx("span",{className:"font-medium text-[var(--color-wi-text)]",children:f(t.requested.start_at,t.requested.end_at)})]}),r.jsx("p",{className:"mt-1 text-xs text-[var(--color-wi-text-light)]",children:y(t.kind)})]}),a&&r.jsx("button",{type:"button","aria-label":"Dismiss conflict context",title:"Start a fresh search",onClick:a,className:"shrink-0 rounded p-0.5 text-[var(--color-wi-text-light)] transition-colors duration-150 hover:text-[var(--color-wi-text)] motion-reduce:transition-none",children:"✕"})]})})}g.__docgenInfo={description:"",methods:[],displayName:"ConflictContextCard",props:{context:{required:!0,tsType:{name:"signature",type:"object",raw:`{
  details: ConflictDetails;
  teacherName?: string;
  roomName?: string;
  studentName?: string;
  studentId?: string;
}`,signature:{properties:[{key:"details",value:{name:"ConflictDetails",required:!0}},{key:"teacherName",value:{name:"string",required:!1}},{key:"roomName",value:{name:"string",required:!1}},{key:"studentName",value:{name:"string",required:!1}},{key:"studentId",value:{name:"string",required:!1}}]}},description:""},coursesById:{required:!0,tsType:{name:"Map",elements:[{name:"string"},{name:"Course"}],raw:"Map<string, Course>"},description:""},onDismiss:{required:!1,tsType:{name:"signature",type:"function",raw:"() => void",signature:{arguments:[],return:{name:"void"}}},description:"Renders an ✕ that hands the page back to a blank search."}}};const T={title:"Scheduling/ConflictContextCard",component:g,parameters:{layout:"centered",docs:{description:{component:"Carried-over conflict context for the Slot Finder landing. The blocked availability strip links here with query params; the card restates the reason so the search page never loses the context of what went wrong."}}},decorators:[e=>r.jsx("div",{className:"w-[40rem]",children:e()})]},l=new Map([["course-1",{id:"course-1",version:1,code:"MATH-101",name:"Calculus",primary_teacher_id:"teacher-1"}]]);function p(e){const o=N(new URLSearchParams(e));if(!o)throw new Error(`invalid story query: ${e}`);return o}const h=["course_id=course-1","teacher_id=teacher-1","room_id=room-1","room=Room+101","teacher=Teacher+One","start_at=2026-06-01T02%3A00%3A00Z","end_at=2026-06-01T04%3A00%3A00Z"].join("&"),S=["course_id=course-1","teacher_id=teacher-1","start_at=2026-06-01T02%3A00%3A00Z","end_at=2026-06-01T04%3A00%3A00Z","student_count=3","student_id=st1","student=Ariya+S."].join("&"),c={name:"Room overlap — names the room",args:{context:p(`kind=room_overlap&${h}`),coursesById:l}},d={name:"Teacher overlap — names the teacher",args:{context:p(`kind=teacher_overlap&${h}`),coursesById:l}},m={name:"Student overlap — counts the students",args:{context:p(`kind=student_overlap&${S}`),coursesById:l}},u={name:"Teacher not assigned to course",args:{context:p(`kind=teacher_not_assigned_to_course&${h}`),coursesById:l}};c.parameters={...c.parameters,docs:{...c.parameters?.docs,source:{originalSource:`{
  name: "Room overlap — names the room",
  args: {
    context: contextFrom(\`kind=room_overlap&\${BASE_QUERY}\`),
    coursesById: COURSES
  }
}`,...c.parameters?.docs?.source}}};d.parameters={...d.parameters,docs:{...d.parameters?.docs,source:{originalSource:`{
  name: "Teacher overlap — names the teacher",
  args: {
    context: contextFrom(\`kind=teacher_overlap&\${BASE_QUERY}\`),
    coursesById: COURSES
  }
}`,...d.parameters?.docs?.source}}};m.parameters={...m.parameters,docs:{...m.parameters?.docs,source:{originalSource:`{
  name: "Student overlap — counts the students",
  args: {
    context: contextFrom(\`kind=student_overlap&\${STUDENT_QUERY}\`),
    coursesById: COURSES
  }
}`,...m.parameters?.docs?.source}}};u.parameters={...u.parameters,docs:{...u.parameters?.docs,source:{originalSource:`{
  name: "Teacher not assigned to course",
  args: {
    context: contextFrom(\`kind=teacher_not_assigned_to_course&\${BASE_QUERY}\`),
    coursesById: COURSES
  }
}`,...u.parameters?.docs?.source}}};const R=["RoomOverlap","TeacherOverlap","StudentOverlap","TeacherNotAssigned"];export{c as RoomOverlap,m as StudentOverlap,u as TeacherNotAssigned,d as TeacherOverlap,R as __namedExportsOrder,T as default};
