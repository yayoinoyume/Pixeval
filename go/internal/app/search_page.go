package app

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// The illustration search's orders and match targets, as the app API names
// them.
const (
	searchSortDateDesc = "date_desc"
	searchSortDateAsc  = "date_asc"
	searchSortPopular  = "popular_desc"

	searchTargetPartial      = "partial_match_for_tags"
	searchTargetExact        = "exact_match_for_tags"
	searchTargetTitleCaption = "title_and_caption"
)

// searchSortChoices are the order labels the search page shows.
func (a *App) searchSortChoices() []string {
	return []string{
		a.t("search.sortDateDesc"),
		a.t("search.sortDateAsc"),
		a.t("search.sortPopular"),
	}
}

// searchSortLabel is the order label the search page shows for a saved sort.
func (a *App) searchSortLabel(sort string) string {
	switch sort {
	case searchSortDateAsc:
		return a.t("search.sortDateAsc")
	case searchSortPopular:
		return a.t("search.sortPopular")
	default:
		return a.t("search.sortDateDesc")
	}
}

// searchSortValue is the sort a label stands for.
func (a *App) searchSortValue(label string) string {
	switch label {
	case a.t("search.sortDateAsc"):
		return searchSortDateAsc
	case a.t("search.sortPopular"):
		return searchSortPopular
	default:
		return searchSortDateDesc
	}
}

// searchTargetChoices are the match labels the search page shows.
func (a *App) searchTargetChoices() []string {
	return []string{
		a.t("search.targetPartial"),
		a.t("search.targetExact"),
		a.t("search.targetTitleCaption"),
	}
}

// searchTargetLabel is the match label the search page shows for a saved
// target.
func (a *App) searchTargetLabel(target string) string {
	switch target {
	case searchTargetExact:
		return a.t("search.targetExact")
	case searchTargetTitleCaption:
		return a.t("search.targetTitleCaption")
	default:
		return a.t("search.targetPartial")
	}
}

// searchTargetValue is the match target a label stands for.
func (a *App) searchTargetValue(label string) string {
	switch label {
	case a.t("search.targetExact"):
		return searchTargetExact
	case a.t("search.targetTitleCaption"):
		return searchTargetTitleCaption
	default:
		return searchTargetPartial
	}
}

// searchUser is the search page's user tab, after the illustration and novel
// ones, and searchSauce its reverse image search tab.
const (
	searchUser  = searchNovel + 1
	searchSauce = searchUser + 1
)

// searchDateLayout is the date format the advanced filter accepts.
const searchDateLayout = "2006-01-02"

// searchOptions builds the illustration search's filters from the page. A
// blank date or size is no limit; a bad one is an error and the search is not
// sent.
func (a *App) searchOptions(word string) (pixiv.SearchOptions, error) {
	opts := pixiv.SearchOptions{Word: word, Sort: a.searchSort, Target: a.searchTarget}
	start, err := a.searchDate(a.searchStartDate)
	if err != nil {
		return opts, err
	}
	end, err := a.searchDate(a.searchEndDate)
	if err != nil {
		return opts, err
	}
	opts.StartDate, opts.EndDate = start, end
	if opts.MinWidth, err = a.searchSize(a.searchMinWidth); err != nil {
		return opts, err
	}
	if opts.MaxWidth, err = a.searchSize(a.searchMaxWidth); err != nil {
		return opts, err
	}
	if opts.MinHeight, err = a.searchSize(a.searchMinHeight); err != nil {
		return opts, err
	}
	if opts.MaxHeight, err = a.searchSize(a.searchMaxHeight); err != nil {
		return opts, err
	}
	return opts, nil
}

// searchDate reads one typed date, blank for no limit.
func (a *App) searchDate(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	if _, err := time.Parse(searchDateLayout, text); err != nil {
		return "", errors.New(a.t("search.invalidDate"))
	}
	return text, nil
}

// searchSize reads one typed size, blank or 0 for no limit.
func (a *App) searchSize(text string) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 0 {
		return 0, errors.New(a.t("search.invalidSize"))
	}
	return n, nil
}

// searchFilterFields draws the illustration search's date and size filters.
// A date is YYYY-MM-DD and a size is a whole number.
func (a *App) searchFilterFields(c *ui.Context, t tokens) {
	fields := []struct {
		label       string
		value       *string
		width       float32
		placeholder string
	}{
		{a.t("search.startDate"), &a.searchStartDate, 140, "YYYY-MM-DD"},
		{a.t("search.endDate"), &a.searchEndDate, 140, "YYYY-MM-DD"},
		{a.t("search.minWidth"), &a.searchMinWidth, 84, ""},
		{a.t("search.maxWidth"), &a.searchMaxWidth, 84, ""},
		{a.t("search.minHeight"), &a.searchMinHeight, 84, ""},
		{a.t("search.maxHeight"), &a.searchMaxHeight, 84, ""},
	}
	ui.Column(c).Padding(t.SpaceL, contentPadding, 0, contentPadding).Gap(t.SpaceS).Children(func() {
		ui.Row(c).Gap(t.SpaceM).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.t("filter.title")).FontSize(t.TextTiny).TextColor(t.TextMuted)
			ui.Box(c).Height(1).Grow(1).Background(t.Border)
		})
		ui.Row(c).Wrap().Gap(t.SpaceM).AlignItems(ui.End).Children(func() {
			for _, f := range fields {
				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, f.label).FontSize(t.TextMicro).TextColor(t.TextMuted)
					ui.TextInput(c, f.value).Width(f.width).Height(32).
						Placeholder(f.placeholder).Label(f.label)
				})
			}
		})
		if a.searchErr != "" {
			ui.Row(c).Children(func() {
				ui.Text(c, a.searchErr).FontSize(t.TextSmall).TextColor(t.Danger)
			})
		}
	})
}

// searchHistoryBar draws the words searched before under the box. A chip
// searches its word again; each has its own delete button, and one clears the
// lot.
func (a *App) searchHistoryBar(c *ui.Context, t tokens) {
	if a.searchHistory == nil {
		return
	}
	words := a.searchHistory.Snapshot()
	if len(words) == 0 {
		return
	}
	ui.Row(c).Wrap().Gap(t.SpaceS).Padding(t.SpaceS, contentPadding, 0, contentPadding).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.t("search.history")).FontSize(t.TextTiny).TextColor(t.TextMuted)
		for _, word := range words {
			a.searchHistoryChip(c, t, word)
		}
		if ui.Button(c, a.t("search.clearHistory")).FontSize(t.TextTiny).Clicked() {
			a.clearSearchHistory()
		}
	})
}

// searchHistoryChip is one searched word and its delete button.
func (a *App) searchHistoryChip(c *ui.Context, t tokens, word string) {
	ui.Row(c).Gap(0).AlignItems(ui.Center).Children(func() {
		b := ui.ButtonBase(c).Radius(t.RadiusS).Padding(t.SpaceXS, t.SpaceS).Label(word)
		if b.Hovered() {
			b = b.Background(t.SurfaceAlt)
		}
		if b.Clicked() {
			a.search.query = word
			a.startSearch(a.search)
		}
		b.Children(func() {
			ui.Text(c, word).FontSize(t.TextSmall).FontWeight(500).TextColor(t.Text)
		})
		label := a.tf("search.deleteHistory", word)
		if iconButton(c, iconClose, label, 22, t.TextMuted, t.SurfaceAlt).Clicked() {
			a.removeSearchWord(word)
		}
	})
}

// loadTrendingTags starts the one load of the search page's trending tags,
// the first time its box draws. The tags are a shortcut, not a result: a
// failure leaves the row out and says nothing.
func (a *App) loadTrendingTags() {
	if a.trendingLoaded || a.trendingFn == nil {
		return
	}
	a.trendingLoaded = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
		defer cancel()
		tags, err := a.trendingFn(ctx)
		if err != nil {
			log.Printf("search: trending tags: %v", err)
			return
		}
		a.searchSideMu.Lock()
		a.searchTags = tags
		a.searchSideMu.Unlock()
		a.update(func() {})
	}()
}

// trendingTagsBar draws the tags Pixiv lists as trending under the search box.
// A chip searches its tag.
func (a *App) trendingTagsBar(c *ui.Context, t tokens) {
	a.searchSideMu.Lock()
	tags := a.searchTags
	a.searchSideMu.Unlock()
	if len(tags) == 0 {
		return
	}
	ui.Row(c).Wrap().Gap(t.SpaceS).Padding(t.SpaceL, contentPadding, 0, contentPadding).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.t("search.trending")).FontSize(t.TextTiny).TextColor(t.TextMuted)
		for _, tag := range tags {
			b := ui.ButtonBase(c).Radius(t.RadiusS).Padding(t.SpaceXS, t.SpaceS).Label(tag.Tag)
			if b.Hovered() {
				b = b.Background(t.SurfaceAlt)
			}
			if b.Clicked() {
				a.search.query = tag.Tag
				a.startSearch(a.search)
			}
			b.Children(func() {
				ui.Text(c, tag.Tag).FontSize(t.TextSmall).FontWeight(500).TextColor(t.Text)
			})
		}
	})
}

// linkErrorBar draws why the last pasted Pixiv link did not open.
func (a *App) linkErrorBar(c *ui.Context, t tokens) {
	a.searchSideMu.Lock()
	msg := a.searchLinkErr
	a.searchSideMu.Unlock()
	if msg == "" {
		return
	}
	ui.Row(c).Padding(t.SpaceXS, contentPadding, 0, contentPadding).Children(func() {
		ui.Text(c, msg).FontSize(t.TextSmall).TextColor(t.Danger)
	})
}

// searchPage draws the search box and its results.
func (a *App) searchPage(c *ui.Context) {
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	st := a.search
	t := themeOf(c)
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		if a.searchKind != searchSauce {
			ui.Row(c).Gap(t.SpaceS).Padding(t.SpaceL, contentPadding, 0, contentPadding).
				AlignItems(ui.Center).Children(func() {
				box := ui.TextInput(c, &st.query).Grow(1).
					Placeholder(a.t("search.placeholder")).Label(a.t("search.placeholder"))
				if st.focusQuery {
					st.focusQuery = false
					box = box.Focus()
				}
				submitted := box.Submitted()
				clicked := ui.PrimaryButton(c, a.t("search.action")).Clicked()
				if submitted || clicked {
					a.startSearch(st)
				}
			})
			a.loadTrendingTags()
			a.linkErrorBar(c, t)
			a.trendingTagsBar(c, t)
			a.searchHistoryBar(c, t)
		}
		ui.Row(c).Padding(t.SpaceXL, contentPadding, 0, contentPadding).Children(func() {
			textTabs(c, t, &a.searchKind, a.t("search.illust"), a.t("search.novel"),
				a.t("search.user"), a.t("search.sauce"))
		})
		if a.searchKind == searchSauce {
			a.saucePage(c, t)
			return
		}
		if a.searchKind == searchIllust {
			ui.Row(c).Gap(t.SpaceS).Padding(t.SpaceM, contentPadding, 0, contentPadding).
				AlignItems(ui.Center).Children(func() {
				sort := a.searchSortLabel(a.searchSort)
				if ui.Select(c, &sort, a.searchSortChoices()).Width(200).Height(36).Label(a.t("search.sort")).Changed() {
					a.searchSort = a.searchSortValue(sort)
				}
				target := a.searchTargetLabel(a.searchTarget)
				if ui.Select(c, &target, a.searchTargetChoices()).Width(260).Height(36).Label(a.t("search.target")).Changed() {
					a.searchTarget = a.searchTargetValue(target)
				}
			})
			a.searchFilterFields(c, t)
		}
		if !a.searchArmed() {
			emptyState(c, iconSearch, a.t("search.hint"), "", nil)
			return
		}
		if a.searchKind == searchNovel {
			a.novelResults(c)
			return
		}
		if a.searchKind == searchUser {
			a.userListPage(c, a.searchUsers, a.t("user.noSearchResults"), "common.loadFailed")
			return
		}
		a.browsePage(c, st)
	})
}
