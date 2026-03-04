# Form Components

A complete collection of form input components extracted from the terminal dashboard, built with React 19 and styled to match the terminal UI aesthetic.

## Components Overview

All form components are **controlled components** following React best practices. They use the value/onChange pattern and include PropTypes validation and JSDoc documentation.

### 1. Input
Text input field with terminal styling.

```jsx
import { Input } from './components/forms';

<Input
  type="text"
  value={value}
  onChange={(e) => setValue(e.target.value)}
  placeholder="Enter value..."
/>
```

**Props:**
- `type` - Input type (text, password, email, url, etc.)
- `value` - Current value (controlled)
- `onChange` - Change handler function
- `placeholder` - Placeholder text
- `className` - Additional CSS classes
- `disabled` - Disabled state
- `id` - Input ID attribute
- `name` - Input name attribute

### 2. CustomDropdown
Custom-styled dropdown replacing native select elements. Features smooth animations, checkmarks on selected items, and click-outside-to-close functionality.

```jsx
import { CustomDropdown } from './components/forms';

const options = [
  { value: '10', label: '10 seconds' },
  { value: '30', label: '30 seconds' },
  { value: '60', label: '1 minute' }
];

<CustomDropdown
  options={options}
  value={selected}
  onChange={(value) => setSelected(value)}
  placeholder="Select an option..."
/>
```

**Props:**
- `options` - Array of `{ value, label }` objects
- `value` - Currently selected value
- `onChange` - Change handler (receives selected value)
- `id` - Dropdown ID attribute
- `className` - Additional CSS classes
- `placeholder` - Text shown when no selection

**Features:**
- Toggle open/close on click
- Close on outside click
- Animated arrow rotation
- Checkmark on active item
- Keyboard accessible

### 3. Checkbox
Custom-styled checkbox with label.

```jsx
import { Checkbox } from './components/forms';

<Checkbox
  label="Enable notifications"
  checked={enabled}
  onChange={(e) => setEnabled(e.target.checked)}
/>
```

**Props:**
- `label` - Label text
- `checked` - Checked state
- `onChange` - Change handler function
- `id` - Checkbox ID attribute
- `disabled` - Disabled state
- `ariaLabel` - ARIA label for accessibility

### 4. Radio
Custom-styled radio button with label.

```jsx
import { Radio } from './components/forms';

<Radio
  label="Option 1"
  name="choices"
  value="option1"
  checked={selected === 'option1'}
  onChange={(e) => setSelected(e.target.value)}
/>
```

**Props:**
- `label` - Label text
- `name` - Radio group name (required)
- `checked` - Checked state
- `onChange` - Change handler function
- `value` - Radio button value (required)
- `disabled` - Disabled state
- `id` - Radio ID attribute

### 5. ToggleSwitch
Animated on/off toggle switch.

```jsx
import { ToggleSwitch } from './components/forms';

<ToggleSwitch
  checked={enabled}
  onChange={(e) => setEnabled(e.target.checked)}
  label="Enable feature"
/>
```

**Props:**
- `checked` - Checked state
- `onChange` - Change handler function
- `id` - Toggle ID attribute
- `label` - Optional label text
- `disabled` - Disabled state

**Features:**
- Smooth slider animation
- 60px wide switch
- Green background when active

### 6. FormGroup
Wrapper component for form fields that includes label, input, and helper/error text.

```jsx
import { FormGroup, Input } from './components/forms';

<FormGroup
  label="Email Address"
  helperText="Enter your email"
  required
>
  <Input value={email} onChange={setEmail} />
</FormGroup>
```

**Props:**
- `label` - Label text
- `children` - Form field element(s)
- `helperText` - Helper text below field
- `error` - Error message (replaces helper text)
- `required` - Shows required asterisk

### 7. Label
Form label with optional required indicator.

```jsx
import { Label } from './components/forms';

<Label htmlFor="email" required>
  Email Address
</Label>
<Input id="email" ... />
```

**Props:**
- `children` - Label content
- `htmlFor` - ID of associated form element
- `required` - Shows required asterisk

### 8. SearchInput
Search/filter input field with search icon (uses Lucide React).

```jsx
import { SearchInput } from './components/forms';

<SearchInput
  value={searchTerm}
  onChange={(e) => setSearchTerm(e.target.value)}
  placeholder="Search routes..."
/>
```

**Props:**
- `value` - Current search value
- `onChange` - Change handler function
- `placeholder` - Placeholder text

**Features:**
- Search icon positioned on left
- Icon from Lucide React
- Full width by default

### 9. TextArea
Multi-line text input field.

```jsx
import { TextArea } from './components/forms';

<TextArea
  value={description}
  onChange={(e) => setDescription(e.target.value)}
  placeholder="Enter description..."
  rows={5}
/>
```

**Props:**
- `value` - Current value (controlled)
- `onChange` - Change handler function
- `placeholder` - Placeholder text
- `rows` - Number of visible rows (default: 4)
- `className` - Additional CSS classes
- `disabled` - Disabled state
- `id` - TextArea ID attribute
- `name` - TextArea name attribute

### 10. Select
Wrapper for native select element with consistent styling.

```jsx
import { Select } from './components/forms';

const options = [
  { value: 'option1', label: 'Option 1' },
  { value: 'option2', label: 'Option 2' }
];

<Select
  options={options}
  value={selected}
  onChange={(e) => setSelected(e.target.value)}
/>
```

**Props:**
- `options` - Array of `{ value, label }` objects
- `value` - Currently selected value
- `onChange` - Change handler function
- `className` - Additional CSS classes
- `disabled` - Disabled state
- `id` - Select ID attribute
- `name` - Select name attribute

**Note:** Use `CustomDropdown` for better visual control and UX. This component is for cases where native select is preferred (e.g., mobile optimization).

## Styling

All components use CSS classes from the dashboard's terminal theme:

- `.input` - Base input styling
- `.checkbox`, `.radio` - Custom checkbox/radio styling
- `.checkbox-label`, `.radio-label` - Label containers
- `.toggle-switch`, `.toggle-slider` - Toggle switch styling
- `.custom-dropdown` - Custom dropdown container
- `.search-input` - Search input styling

Styles are defined in the web styles directory and match the terminal aesthetic with:
- Monospace font (JetBrains Mono)
- Green borders and text
- Dark backgrounds
- Uppercase labels
- Terminal-style focus indicators

## Accessibility

All components include:
- Proper ARIA attributes
- Keyboard navigation support
- Focus visible states
- Screen reader support
- Semantic HTML

## Usage Example - Complete Form

```jsx
import React, { useState } from 'react';
import {
  Input,
  CustomDropdown,
  Checkbox,
  Radio,
  ToggleSwitch,
  FormGroup,
  SearchInput,
  TextArea,
} from './components/forms';

function SettingsForm() {
  const [email, setEmail] = useState('');
  const [interval, setInterval] = useState('30');
  const [notifications, setNotifications] = useState(true);
  const [theme, setTheme] = useState('dark');
  const [enabled, setEnabled] = useState(false);
  const [search, setSearch] = useState('');
  const [notes, setNotes] = useState('');

  const intervalOptions = [
    { value: '10', label: '10 seconds' },
    { value: '30', label: '30 seconds' },
    { value: '60', label: '1 minute' },
  ];

  return (
    <form>
      <FormGroup label="Email" helperText="Your email address" required>
        <Input
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="user@example.com"
        />
      </FormGroup>

      <FormGroup label="Refresh Interval">
        <CustomDropdown
          options={intervalOptions}
          value={interval}
          onChange={setInterval}
        />
      </FormGroup>

      <FormGroup>
        <Checkbox
          label="Enable email notifications"
          checked={notifications}
          onChange={(e) => setNotifications(e.target.checked)}
        />
      </FormGroup>

      <FormGroup label="Theme">
        <Radio
          label="Dark"
          name="theme"
          value="dark"
          checked={theme === 'dark'}
          onChange={(e) => setTheme(e.target.value)}
        />
        <Radio
          label="Light"
          name="theme"
          value="light"
          checked={theme === 'light'}
          onChange={(e) => setTheme(e.target.value)}
        />
      </FormGroup>

      <FormGroup>
        <ToggleSwitch
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          label="Enable feature"
        />
      </FormGroup>

      <FormGroup label="Search">
        <SearchInput
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search settings..."
        />
      </FormGroup>

      <FormGroup label="Notes">
        <TextArea
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          placeholder="Additional notes..."
          rows={4}
        />
      </FormGroup>
    </form>
  );
}
```

## File Structure

```
forms/
├── Input.jsx               # Text input component
├── CustomDropdown.jsx      # Custom dropdown component
├── Checkbox.jsx            # Checkbox component
├── Radio.jsx              # Radio button component
├── ToggleSwitch.jsx       # Toggle switch component
├── FormGroup.jsx          # Form field wrapper
├── Label.jsx              # Form label component
├── SearchInput.jsx        # Search input with icon
├── TextArea.jsx           # Multi-line text input
├── Select.jsx             # Native select wrapper
├── index.js               # Barrel exports
└── README.md              # This file
```

## Dependencies

- **React 19** - Core framework
- **PropTypes** - Runtime type checking
- **Lucide React** - Icons (SearchInput only)

## Best Practices

1. **Always use controlled components** - Pass value and onChange to all inputs
2. **Use FormGroup for structure** - Wraps inputs with labels and helper text
3. **Prefer CustomDropdown over Select** - Better UX and styling control
4. **Add ARIA labels** - Especially for inputs without visible labels
5. **Use semantic HTML** - Proper label/input associations with htmlFor/id
6. **Handle disabled states** - All components support disabled prop
7. **Provide placeholders** - Help users understand expected input

## Theme Integration

All form components automatically adapt to the terminal theme system:
- Use CSS custom properties for colors
- Support all 9 dashboard themes (including Pipboy)
- Maintain WCAG AA contrast compliance
- Respond to font size settings (small/medium/large)

## Testing

Components can be tested with React Testing Library:

```jsx
import { render, screen, fireEvent } from '@testing-library/react';
import { Input } from './components/forms';

test('input updates on change', () => {
  const handleChange = jest.fn();
  render(<Input value="" onChange={handleChange} />);

  const input = screen.getByRole('textbox');
  fireEvent.change(input, { target: { value: 'test' } });

  expect(handleChange).toHaveBeenCalled();
});
```

## Future Enhancements

- [ ] Add validation support to FormGroup
- [ ] Create DateInput component
- [ ] Add NumberInput with increment/decrement
- [ ] Create FileInput component
- [ ] Add ColorPicker component
- [ ] Support custom icons in Input
- [ ] Add input masks support
- [ ] Create multi-select CustomDropdown variant
