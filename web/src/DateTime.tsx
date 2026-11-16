import {
  Calendar,
  DateField,
  DatePicker,
  Label,
  Button,
  Select,
  ListBox,
  Popover,
} from "@heroui/react";
import { useState } from "react";
import { CalendarDays, X } from "lucide-react";
import {
  getLocalTimeZone,
  now,
  parseAbsoluteToLocal,
  type DateValue,
} from "@internationalized/date";

export function DateTime({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  const date = value ? parseAbsoluteToLocal(value) : null;
  return (
    <div className="min-w-0">
      <DatePicker
        className="w-full"
        aria-label={label}
        value={date}
        placeholderValue={now(getLocalTimeZone())}
        granularity="second"
        hourCycle={24}
        hideTimeZone
        onChange={(v: DateValue | null) =>
          onChange(v ? v.toDate(getLocalTimeZone()).toISOString() : "")
        }
      >
        <Label>{label}</Label>
        <DateField.Group fullWidth>
          <DateField.Input>
            {(segment) => <DateField.Segment segment={segment} />}
          </DateField.Input>
          <DateField.Suffix>
            {value && (
              <Button
                slot={null}
                size="sm"
                isIconOnly
                variant="ghost"
                aria-label={"清除" + label}
                onPress={() => onChange("")}
              >
                <X size={13} />
              </Button>
            )}
            <DatePicker.Trigger>
              <DatePicker.TriggerIndicator />
            </DatePicker.Trigger>
          </DateField.Suffix>
        </DateField.Group>
        <DatePicker.Popover>
          <Calendar aria-label={label}>
            <Calendar.Header>
              <Calendar.YearPickerTrigger>
                <Calendar.YearPickerTriggerHeading />
                <Calendar.YearPickerTriggerIndicator />
              </Calendar.YearPickerTrigger>
              <Calendar.NavButton slot="previous" />
              <Calendar.NavButton slot="next" />
            </Calendar.Header>
            <Calendar.Grid>
              <Calendar.GridHeader>
                {(day) => <Calendar.HeaderCell>{day}</Calendar.HeaderCell>}
              </Calendar.GridHeader>
              <Calendar.GridBody>
                {(date) => <Calendar.Cell date={date} />}
              </Calendar.GridBody>
            </Calendar.Grid>
            <Calendar.YearPickerGrid>
              <Calendar.YearPickerGridBody>
                {({ year }) => <Calendar.YearPickerCell year={year} />}
              </Calendar.YearPickerGridBody>
            </Calendar.YearPickerGrid>
          </Calendar>
        </DatePicker.Popover>
      </DatePicker>
    </div>
  );
}
export function MultiChoice({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string[];
  onChange: (v: string[]) => void;
  options: { id: string; label: string }[];
}) {
  return (
    <Select
      aria-label={label}
      placeholder="请选择"
      selectionMode="multiple"
      value={value}
      onChange={(v) => onChange(v as string[])}
    >
      <Label>{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox selectionMode="multiple">
          {options.map((o) => (
            <ListBox.Item key={o.id} id={o.id} textValue={o.label}>
              {o.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  );
}

export function TimeRange({
  label,
  start,
  end,
  onChange,
}: {
  label: string;
  start: string;
  end: string;
  onChange: (start: string, end: string) => void;
}) {
  const [open, setOpen] = useState(false),
    [from, setFrom] = useState(start),
    [to, setTo] = useState(end);
  const invalid = !!from && !!to && new Date(from) > new Date(to);
  const format = (value: string) =>
    value
      ? new Intl.DateTimeFormat("zh-CN", {
          month: "2-digit",
          day: "2-digit",
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
          hour12: false,
        }).format(new Date(value))
      : "不限";
  const summary =
    start || end ? format(start) + " — " + format(end) : "全部时间";
  const preset = (days: number) => {
    const endDate = new Date();
    endDate.setHours(23, 59, 59, 999);
    const startDate = new Date();
    startDate.setDate(startDate.getDate() - days + 1);
    startDate.setHours(0, 0, 0, 0);
    onChange(startDate.toISOString(), endDate.toISOString());
    setOpen(false);
  };
  return (
    <div className="min-w-0">
      <Label className="mb-1 block text-xs">{label}</Label>
      <div className="flex items-center gap-1">
        <Popover
          isOpen={open}
          onOpenChange={(next) => {
            if (next) {
              setFrom(start);
              setTo(end);
            }
            setOpen(next);
          }}
        >
          <Button
            variant="secondary"
            className="min-w-0 flex-1 justify-start px-3 font-normal"
            aria-label={label + "：" + summary}
          >
            <CalendarDays size={15} className="shrink-0" />
            <span className="truncate text-xs tabular-nums">{summary}</span>
          </Button>
          <Popover.Content
            placement="bottom start"
            className="w-[min(620px,calc(100vw-2rem))]"
          >
            <Popover.Dialog className="space-y-4 p-3">
              <Popover.Heading>{label}</Popover.Heading>
              <div className="grid gap-3 sm:grid-cols-2">
                <DateTime label="开始时间" value={from} onChange={setFrom} />
                <DateTime label="结束时间" value={to} onChange={setTo} />
              </div>
              <p className="text-xs text-muted">
                使用浏览器当前时区，可精确到秒，支持单侧时间范围。
              </p>
              {invalid && (
                <p role="alert" className="text-xs text-danger">
                  结束时间不能早于开始时间
                </p>
              )}
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex gap-1">
                  {[
                    ["今天", 1],
                    ["近7天", 7],
                    ["近30天", 30],
                  ].map(([name, days]) => (
                    <Button
                      key={name}
                      size="sm"
                      variant="tertiary"
                      onPress={() => preset(Number(days))}
                    >
                      {name}
                    </Button>
                  ))}
                </div>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="tertiary"
                    onPress={() => setOpen(false)}
                  >
                    取消
                  </Button>
                  <Button
                    size="sm"
                    isDisabled={invalid}
                    onPress={() => {
                      onChange(from, to);
                      setOpen(false);
                    }}
                  >
                    应用
                  </Button>
                </div>
              </div>
            </Popover.Dialog>
          </Popover.Content>
        </Popover>
        {(start || end) && (
          <Button
            size="sm"
            isIconOnly
            variant="ghost"
            aria-label={"清除" + label}
            onPress={() => onChange("", "")}
          >
            <X size={15} />
          </Button>
        )}
      </div>
    </div>
  );
}
