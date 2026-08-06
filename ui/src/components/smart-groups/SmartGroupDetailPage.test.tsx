import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import SmartGroupExpressionBuilder from './SmartGroupExpressionBuilder';
import type { SmartGroupExpression } from './SmartGroupTypes';

vi.mock('@/components/shared/Toast', () => ({
  useToast: () => ({
    showToast: vi.fn(),
  }),
}));

describe('SmartGroupExpressionBuilder', () => {
  it('renders with default empty expression', () => {
    const onChange = vi.fn();
    render(<SmartGroupExpressionBuilder onExpressionChange={onChange} />);
    expect(screen.getByText('Expression Builder')).toBeInTheDocument();
  });

  it('renders with initial expression', () => {
    const initialExpression: SmartGroupExpression = {
      condition: 'AND',
      filters: [
        { field: 'os', op: 'startswith', value: 'Linux' },
      ],
    };
    const onChange = vi.fn();
    render(
      <SmartGroupExpressionBuilder
        initialExpression={initialExpression}
        onExpressionChange={onChange}
      />
    );
    expect(screen.getByText('Expression Builder')).toBeInTheDocument();
  });

  it('allows adding a new filter', () => {
    const initialExpression: SmartGroupExpression = {
      condition: 'AND',
      filters: [],
    };
    const onChange = vi.fn();
    render(
      <SmartGroupExpressionBuilder
        initialExpression={initialExpression}
        onExpressionChange={onChange}
      />
    );
    fireEvent.click(screen.getByText('Add Filter'));
    expect(onChange).toHaveBeenCalled();
  });

  it('allows removing a filter', () => {
    const initialExpression: SmartGroupExpression = {
      condition: 'AND',
      filters: [
        { field: 'os', op: 'eq', value: 'Linux' },
        { field: 'platform', op: 'eq', value: 'linux' },
      ],
    };
    const onChange = vi.fn();
    render(
      <SmartGroupExpressionBuilder
        initialExpression={initialExpression}
        onExpressionChange={onChange}
      />
    );
    const removeButtons = screen.getAllByTitle('Remove filter');
    if (removeButtons.length > 0) {
      fireEvent.click(removeButtons[0]);
      expect(onChange).toHaveBeenCalled();
    }
  });

  it('switches to JSON view mode', () => {
    const onChange = vi.fn();
    render(<SmartGroupExpressionBuilder onExpressionChange={onChange} />);
    const jsonBtn = screen.getByText('JSON');
    expect(jsonBtn).toBeInTheDocument();
    fireEvent.click(jsonBtn);
    expect(screen.getByText('JSON')).toHaveClass('bg-blue-600');
  });

  it('renders field and operator selectors when filters exist', () => {
    const initialExpression: SmartGroupExpression = {
      condition: 'AND',
      filters: [
        { field: 'os', op: 'eq', value: 'Linux' },
      ],
    };
    const onChange = vi.fn();
    render(
      <SmartGroupExpressionBuilder
        initialExpression={initialExpression}
        onExpressionChange={onChange}
      />
    );
    const selects = screen.getAllByRole('combobox');
    expect(selects.length).toBeGreaterThanOrEqual(2);
  });

  it('logic operator changes expression condition', () => {
    const onChange = vi.fn();
    render(<SmartGroupExpressionBuilder onExpressionChange={onChange} />);
    const select = screen.getByRole('combobox') as HTMLSelectElement;
    fireEvent.change(select, { target: { value: 'OR' } });
    expect(onChange).toHaveBeenCalled();
  });

  it('renders add nested group button', () => {
    const onChange = vi.fn();
    render(<SmartGroupExpressionBuilder onExpressionChange={onChange} />);
    expect(screen.getByText('Nested Group')).toBeInTheDocument();
  });
});
