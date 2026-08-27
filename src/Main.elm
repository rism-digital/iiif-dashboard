module Main exposing (VersionState(..), coreStatus, healthStatuses, main, negotiatedVersionState, ruleReferences)

import Browser
import Dict exposing (Dict)
import Domain exposing (CheckResult, Project, ProjectResults, RedirectHop, Registry, ResultsFile, Status(..), statusLabel, statusRank)
import Html exposing (Html, a, button, details, div, footer, h1, h2, header, input, label, main_, p, pre, span, summary, table, tbody, td, text, th, thead, tr)
import Html.Attributes exposing (attribute, autocomplete, class, colspan, for, href, id, placeholder, rel, scope, target, title, type_, value)
import Html.Events exposing (onClick, onInput)
import Http
import ProjectSearch
import RismLogo
import Set exposing (Set)


type alias Model =
    { registry : Maybe Registry
    , snapshots : Maybe ResultsFile
    , loadErrors : List String
    , expanded : Set String
    , searchQuery : String
    }


type Msg
    = GotRegistry (Result Http.Error Registry)
    | GotSnapshots (Result Http.Error ResultsFile)
    | ToggleDetails String
    | SetSearchQuery String


type VersionState
    = VersionResult Status
    | VersionUnavailable String


main : Program () Model Msg
main =
    Browser.element
        { init = init
        , update = update
        , subscriptions = always Sub.none
        , view = view
        }


init : () -> ( Model, Cmd Msg )
init _ =
    ( { registry = Nothing, snapshots = Nothing, loadErrors = [], expanded = Set.empty, searchQuery = "" }
    , Cmd.batch
        [ Http.get { url = "./projects.json", expect = Http.expectJson GotRegistry Domain.registryDecoder }
        , Http.get { url = "./results.json", expect = Http.expectJson GotSnapshots Domain.resultsDecoder }
        ]
    )


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        GotRegistry result ->
            case result of
                Ok registry ->
                    ( { model | registry = Just registry }, Cmd.none )

                Err error ->
                    ( { model | loadErrors = httpError "project registry" error :: model.loadErrors }, Cmd.none )

        GotSnapshots result ->
            case result of
                Ok snapshots ->
                    ( { model | snapshots = Just snapshots }, Cmd.none )

                Err error ->
                    ( { model | loadErrors = httpError "snapshot results" error :: model.loadErrors }, Cmd.none )

        ToggleDetails projectId ->
            ( { model
                | expanded =
                    if Set.member projectId model.expanded then
                        Set.remove projectId model.expanded

                    else
                        Set.insert projectId model.expanded
              }
            , Cmd.none
            )

        SetSearchQuery query ->
            ( { model | searchQuery = query }, Cmd.none )


view : Model -> Html Msg
view model =
    div [ class "app-shell" ]
        [ header [ class "site-header" ]
            [ div [ class "header-inner" ]
                [ div [ class "brand-mark", attribute "aria-hidden" "true" ] [ RismLogo.view ]
                , div []
                    [ h1 [] [ text "IIIF Service Dashboard" ]
                    , p [ class "subtitle" ] [ text "Presentation and Image API compatibility overview" ]
                    ]
                , div [ class "header-links" ]
                    [ a
                        [ class "header-link"
                        , href "./checks.html"
                        ]
                        [ text "Checks explained" ]
                    , a
                        [ class "header-link"
                        , href "https://github.com/rism-digital/iiif-dashboard"
                        , target "_blank"
                        , rel "noopener noreferrer"
                        ]
                        [ text "View on GitHub ↗" ]
                    ]
                ]
            ]
        , main_ [ class "page-shell" ]
            [ div [ class "intro" ]
                [ div []
                    [ h2 [] [ text "Known implementations" ]
                    , p [] [ text "Results are point-in-time observations from the service checker. Failures may be transient." ]
                    , p [] [ text "Purple advisories identify optional improvements; yellow warnings identify concrete interoperability risks." ]
                    ]
                , snapshotStamp model.snapshots
                ]
            , viewErrors model.loadErrors
            , case model.registry of
                Nothing ->
                    div [ class "loading" ] [ span [ class "spinner" ] [], text "Loading service registry…" ]

                Just registry ->
                    dashboardTable model registry.projects
            , p [ class "method-note" ]
                [ text "Our service checker inspects redirects, OPTIONS responses, and exact HTTP headers." ]
            ]
        , footer [ class "site-footer" ]
            [ div [ class "footer-inner" ]
                [ p []
                    [ text "Created by the "
                    , a [ href "https://rism.digital/", target "_blank", rel "noopener noreferrer" ] [ text "RISM Digital Center" ]
                    , text "."
                    ]
                , a [ href "https://rism.online/", target "_blank", rel "noopener noreferrer", class "rism-online-link" ] [ text "Visit RISM Online ↗" ]
                ]
            ]
        ]


snapshotStamp : Maybe ResultsFile -> Html msg
snapshotStamp snapshots =
    case snapshots |> Maybe.andThen .generatedAt of
        Just stamp ->
            div [ class "snapshot-stamp" ] [ span [ class "pulse-dot" ] [], text ("Snapshot " ++ friendlyStamp stamp) ]

        Nothing ->
            div [ class "snapshot-stamp snapshot-empty" ] [ text "No snapshot yet" ]


dashboardTable : Model -> List Project -> Html Msg
dashboardTable model projects =
    let
        filteredProjects =
            List.filter (ProjectSearch.matches model.searchQuery) projects

        resultCount =
            List.length filteredProjects

        totalCount =
            List.length projects
    in
    div [ class "dashboard-region" ]
        [ div [ class "project-search" ]
            [ label [ for "project-search-input" ] [ text "Search projects" ]
            , div [ class "search-input-row" ]
                [ input
                    [ id "project-search-input"
                    , class "search-input"
                    , type_ "search"
                    , placeholder "Try Scotland, Bodleian, or a hostname"
                    , autocomplete False
                    , value model.searchQuery
                    , onInput SetSearchQuery
                    ]
                    []
                , if String.isEmpty model.searchQuery then
                    text ""

                  else
                    button [ class "clear-search", type_ "button", onClick (SetSearchQuery "") ] [ text "Clear" ]
                ]
            , p [ class "search-count", attribute "aria-live" "polite" ]
                [ text
                    ("Showing "
                        ++ String.fromInt resultCount
                        ++ " of "
                        ++ String.fromInt totalCount
                        ++ " projects"
                    )
                ]
            ]
        , if List.isEmpty filteredProjects then
            div [ class "empty-search-results" ]
                [ h2 [] [ text "No matching projects" ]
                , p [] [ text "Try fewer or different keywords." ]
                ]

          else
            div [ class "table-wrap" ]
                [ table [ class "dashboard-table" ]
                    [ thead []
                        [ tr []
                            [ th [ scope "col", class "service-heading" ] [ text "Service" ]
                            , th [ scope "col" ] [ text "Presentation API" ]
                            , th [ scope "col" ] [ text "Image API" ]
                            , th [ scope "col", class "action-heading" ] [ text "Details" ]
                            ]
                        ]
                    , tbody [] (List.concatMap (projectRows model) filteredProjects)
                    ]
                ]
        ]


projectRows : Model -> Project -> List (Html Msg)
projectRows model project =
    let
        displayed =
            if project.skip then
                Dict.empty

            else
                displayedResults model project.id

        snapshotResult =
            snapshotProjectResults model project.id

        checkedState =
            if project.skip then
                Just False

            else
                Maybe.map .checked snapshotResult

        sourceText =
            if project.skip then
                "Checks disabled"

            else
                case snapshotResult of
                    Just result ->
                        if result.checked then
                            "Scheduled snapshot"

                        else
                            "Not checked"

                    Nothing ->
                        "No snapshot"

        isExpanded =
            Set.member project.id model.expanded

        mainRow =
            tr [ class "project-row", id project.id ]
                [ td [ class "service-cell" ]
                    [ div [ class "service-cell-content" ]
                        [ div [ class "service-title-row" ]
                            [ a [ href project.homepage, target "_blank", rel "noopener noreferrer", class "service-name" ] [ text project.name ]
                            , a
                                [ href ("#" ++ project.id)
                                , class "entry-permalink"
                                , title ("Link to " ++ project.name)
                                , attribute "aria-label" ("Link to " ++ project.name)
                                ]
                                [ text "#" ]
                            ]
                        , span [ class "source-label" ] [ text sourceText ]
                        , case project.notes of
                            Just _ ->
                                span [ class "project-note-indicator" ] [ text "Project note available" ]

                            Nothing ->
                                text ""
                        , healthIndicator checkedState project displayed
                        ]
                    ]
                , td []
                    [ if project.skip then
                        unavailableApiSummary "Checks disabled"

                      else
                        case project.manifestUrl of
                            Just _ ->
                                apiSummary "presentation" displayed

                            Nothing ->
                                unavailableApiSummary "No sample manifest"
                    ]
                , td []
                    [ if project.skip then
                        unavailableApiSummary "Checks disabled"

                      else
                        case project.imageInfoUrl of
                            Just _ ->
                                apiSummary "image" displayed

                            Nothing ->
                                unavailableApiSummary "No image sample"
                    ]
                , td [ class "actions-cell" ]
                    [ div [ class "actions-content" ]
                        [ button [ class "details-button", onClick (ToggleDetails project.id), attribute "aria-expanded" (boolString isExpanded) ]
                            [ text
                                (if isExpanded then
                                    "Hide details"

                                 else
                                    "Details"
                                )
                            ]
                        ]
                    ]
                ]

        detailsRow =
            tr [ class "details-row" ]
                [ td [ colspan 4 ] [ detailsPanel project displayed ] ]
    in
    if isExpanded then
        [ mainRow, detailsRow ]

    else
        [ mainRow ]


displayedResults : Model -> String -> Dict String CheckResult
displayedResults model projectId =
    snapshotProjectResults model projectId
        |> Maybe.map .checks
        |> Maybe.withDefault Dict.empty


snapshotProjectResults : Model -> String -> Maybe ProjectResults
snapshotProjectResults model projectId =
    model.snapshots
        |> Maybe.andThen (\results -> findProjectResults projectId results.projects)


findProjectResults : String -> List ProjectResults -> Maybe ProjectResults
findProjectResults projectId projects =
    List.filter (\project -> project.id == projectId) projects |> List.head


primaryHealthGroups : Project -> List ( String, List String )
primaryHealthGroups project =
    [ case project.manifestUrl of
        Just _ ->
            Just
                ( "P"
                , [ "presentation.default"
                  , "presentation.compression"
                  , "presentation.v2"
                  , "presentation.v3"
                  ]
                )

        Nothing ->
            Nothing
    , case project.imageInfoUrl of
        Just _ ->
            Just
                ( "I"
                , [ "image.default"
                  , "image.compression"
                  , "image.v2"
                  , "image.v3"
                  , "image.base-redirect"
                  , "image.response"
                  ]
                )

        Nothing ->
            Nothing
    ]
        |> List.filterMap identity


healthStatuses : Project -> Dict String CheckResult -> List Status
healthStatuses project checks =
    primaryHealthGroups project
        |> List.concatMap Tuple.second
        |> List.map
            (\key ->
                Dict.get key checks
                    |> Maybe.map .status
                    |> Maybe.withDefault Unknown
            )


healthIndicator : Maybe Bool -> Project -> Dict String CheckResult -> Html msg
healthIndicator checkedState project checks =
    let
        count status =
            statuses
                |> List.filter ((==) status)
                |> List.length

        statuses =
            healthStatuses project checks

        passed =
            count Pass

        warnings =
            count Warning

        failed =
            count Fail

        advisories =
            count Advisory

        notTested =
            count Unknown

        scored =
            passed + warnings + failed

        total =
            List.length statuses

        plural amount singular pluralForm =
            if amount == 1 then
                singular

            else
                pluralForm

        summary =
            if project.skip then
                "Health: checks disabled"

            else if checkedState == Just False then
                "Health: not checked"

            else
                "Health: "
                    ++ String.fromInt scored
                    ++ " scored of "
                    ++ String.fromInt total
                    ++ " primary "
                    ++ plural total "request" "requests"
                    ++ " — "
                    ++ String.fromInt passed
                    ++ " passed, "
                    ++ String.fromInt warnings
                    ++ " "
                    ++ plural warnings "warning" "warnings"
                    ++ ", "
                    ++ String.fromInt failed
                    ++ " failed; "
                    ++ String.fromInt advisories
                    ++ " "
                    ++ plural advisories "advisory" "advisories"
                    ++ "; "
                    ++ String.fromInt notTested
                    ++ " not tested. Preflight is shown in details."

        slot key =
            let
                result =
                    Dict.get key checks

                status =
                    result
                        |> Maybe.map .status
                        |> Maybe.withDefault Unknown

                apiName =
                    if String.startsWith "presentation." key then
                        "Presentation API"

                    else
                        "Image API"

                explanation =
                    result
                        |> Maybe.map .summary
                        |> Maybe.andThen
                            (\value ->
                                if String.isEmpty value then
                                    Nothing

                                else
                                    Just value
                            )
                        |> Maybe.withDefault "No result is available for this request."

                tooltip =
                    apiName
                        ++ " — "
                        ++ checkName key
                        ++ " — "
                        ++ statusLabel status
                        ++ ": "
                        ++ explanation

                statusClassName =
                    case status of
                        Pass ->
                            " status-bg-pass"

                        Warning ->
                            " status-bg-warning"

                        Fail ->
                            " status-bg-fail"

                        Advisory ->
                            " status-bg-advisory"

                        Unknown ->
                            " status-bg-unknown"
            in
            span
                [ class ("health-led" ++ statusClassName)
                , attribute "data-tooltip" tooltip
                , attribute "tabindex" "0"
                , attribute "aria-label" tooltip
                ]
                []

        meterGroup ( labelText, keys ) =
            span [ class "health-api-group" ]
                (span [ class "health-api-label", attribute "aria-hidden" "true" ] [ text labelText ] :: List.map slot keys)
    in
    div [ class "project-health", attribute "role" "group", attribute "aria-label" summary ]
        [ span [ class "health-label", attribute "aria-hidden" "true" ] [ text "Health" ]
        , if project.skip then
            span [ class "health-empty" ] [ text "Disabled" ]

          else if checkedState == Just False then
            span [ class "health-empty" ] [ text "Not checked" ]

          else
            span [ class "health-meter" ]
                (List.map meterGroup (primaryHealthGroups project))
        ]


apiSummary : String -> Dict String CheckResult -> Html msg
apiSummary prefix checks =
    let
        aggregate =
            coreStatus prefix checks

        defaultResult =
            Dict.get (prefix ++ ".default") checks

        versionStatus version =
            case defaultResult of
                Just plainResult ->
                    if plainResult.detected |> Maybe.map (String.startsWith version) |> Maybe.withDefault False then
                        VersionResult plainResult.status

                    else
                        negotiatedVersionState prefix version checks

                Nothing ->
                    negotiatedVersionState prefix version checks

        level =
            if prefix == "image" then
                Dict.get "image.default" checks |> Maybe.andThen .detected |> Maybe.withDefault "Level unknown"

            else
                ""
    in
    div [ class "api-summary" ]
        [ statusBadge aggregate
        , div [ class "version-list" ]
            [ miniStatus "v2" (versionStatus "v2")
            , miniStatus "v3" (versionStatus "v3")
            ]
        , if prefix == "image" then
            span [ class "level-label" ] [ text level ]

          else
            text ""
        ]


coreStatus : String -> Dict String CheckResult -> Status
coreStatus prefix checks =
    checks
        |> Dict.toList
        |> List.filter
            (\( key, _ ) ->
                String.startsWith (prefix ++ ".") key
                    && not (String.endsWith ".v2" key)
                    && not (String.endsWith ".v3" key)
                    && not (String.contains "preflight" key)
            )
        |> List.map Tuple.second
        |> List.map
            (\check ->
                if check.status == Advisory then
                    { check | status = Pass }

                else
                    check
            )
        |> List.sortBy (.status >> statusRank >> negate)
        |> List.head
        |> Maybe.map .status
        |> Maybe.withDefault Unknown


unavailableApiSummary : String -> Html msg
unavailableApiSummary reason =
    div [ class "api-summary" ]
        [ statusBadge Unknown
        , span [ class "level-label" ] [ text reason ]
        ]


statusBadge : Status -> Html msg
statusBadge status =
    span [ class ("status-badge status-" ++ statusClass status) ]
        [ span [ class "status-symbol", attribute "aria-hidden" "true" ] [ text (statusIcon status) ]
        , text (statusLabel status)
        ]


negotiatedVersionState : String -> String -> Dict String CheckResult -> VersionState
negotiatedVersionState prefix version checks =
    case Dict.get (prefix ++ "." ++ version) checks of
        Just result ->
            if result.httpStatus == Just 406 then
                VersionUnavailable "HTTP 406"

            else if result.status == Advisory then
                VersionUnavailable
                    (result.detected
                        |> Maybe.map (\detected -> "received " ++ detected)
                        |> Maybe.withDefault "requested representation not returned"
                    )

            else
                VersionResult result.status

        Nothing ->
            VersionResult Unknown


miniStatus : String -> VersionState -> Html msg
miniStatus version state =
    case state of
        VersionResult status ->
            span [ class ("mini-status mini-" ++ statusClass status), title (version ++ ": " ++ statusLabel status) ]
                [ span [ class "mini-dot", attribute "aria-hidden" "true" ] [], text version ]

        VersionUnavailable reason ->
            span [ class "mini-status mini-unavailable", title (version ++ ": Unavailable (" ++ reason ++ ")") ]
                [ span [ class "mini-dot", attribute "aria-hidden" "true" ] [], text version ]


detailsPanel : Project -> Dict String CheckResult -> Html Msg
detailsPanel project checks =
    let
        checkRows prefix =
            checks
                |> Dict.toList
                |> List.filter (\( key, _ ) -> String.startsWith (prefix ++ ".") key && not (String.contains "preflight" key))
                |> List.sortBy
                    (\( key, _ ) ->
                        if String.endsWith ".default" key then
                            "0"

                        else
                            "1" ++ key
                    )
                |> List.map diagnosticRow

        corsRows =
            checks
                |> Dict.toList
                |> List.sortBy Tuple.first
                |> List.map corsDiagnosticRow
    in
    div [ class "details-panel" ]
        [ div [ class "details-header" ]
            [ div [] [ h2 [] [ text "Diagnostic details" ], p [] [ text "Each result describes one request made to the sample endpoint." ] ]
            , div [ class "endpoint-links" ]
                [ case project.manifestUrl of
                    Just manifestUrl ->
                        a [ href manifestUrl, target "_blank", rel "noopener noreferrer" ] [ text "Sample manifest ↗" ]

                    Nothing ->
                        span [ class "missing-sample" ] [ text "No manifest sample configured" ]
                , case project.imageInfoUrl of
                    Just imageInfoUrl ->
                        a [ href imageInfoUrl, target "_blank", rel "noopener noreferrer" ] [ text "Image info.json ↗" ]

                    Nothing ->
                        span [ class "missing-sample" ] [ text "No image sample configured" ]
                ]
            ]
        , case project.notes of
            Just note ->
                div [ class "project-note" ]
                    [ span [ class "project-note-label" ] [ text "Project note" ]
                    , p [] [ text note ]
                    ]

            Nothing ->
                text ""
        , if project.skip then
            p [ class "empty-checks" ] [ text "Automated checks are disabled for this service. Use the sample links above for manual testing." ]

          else
            div [ class "diagnostic-grid" ]
                [ case project.manifestUrl of
                    Just _ ->
                        diagnosticGroup "Presentation API" (checkRows "presentation")

                    Nothing ->
                        div [ class "diagnostic-group" ]
                            [ h2 [] [ text "Presentation API" ]
                            , p [ class "empty-checks" ] [ text "Not tested because this project does not have a sample manifest." ]
                            ]
                , case project.imageInfoUrl of
                    Just _ ->
                        diagnosticGroup "Image API" (checkRows "image")

                    Nothing ->
                        div [ class "diagnostic-group" ]
                            [ h2 [] [ text "Image API" ]
                            , p [ class "empty-checks" ] [ text "Not tested because this project does not yet have a sample image service." ]
                            ]
                , div [ class "cors-group" ] [ diagnosticGroup "CORS and content-negotiation preflight" corsRows ]
                ]
        ]


diagnosticGroup : String -> List (Html msg) -> Html msg
diagnosticGroup heading rows =
    div [ class "diagnostic-group" ]
        [ h2 [] [ text heading ]
        , if List.isEmpty rows then
            p [ class "empty-checks" ] [ text "No results yet." ]

          else
            div [ class "diagnostic-list" ] rows
        ]


diagnosticRow : ( String, CheckResult ) -> Html msg
diagnosticRow ( key, check ) =
    div [ class ("diagnostic-item diagnostic-" ++ statusClass check.status) ]
        [ div [ class "diagnostic-title" ]
            [ span [ class ("diagnostic-dot status-bg-" ++ statusClass check.status), attribute "aria-hidden" "true" ] []
            , span [] [ text (checkName key) ]
            , span [ class "http-code" ] [ text (Maybe.map (\code -> "HTTP " ++ String.fromInt code) check.httpStatus |> Maybe.withDefault "No response") ]
            ]
        , p [] [ text check.summary ]
        , case check.requestAccept of
            Just accept ->
                p [ class "metadata request-metadata" ] [ text ("Request Accept: " ++ accept) ]

            Nothing ->
                if String.endsWith ".default" key then
                    p [ class "metadata request-metadata" ] [ text "Request Accept: not set — plain request" ]

                else
                    text ""
        , case check.contentType of
            Just mediaType ->
                p [ class "metadata" ] [ text ("Response Content-Type: " ++ mediaType) ]

            Nothing ->
                text ""
        , case check.location of
            Just location ->
                p [ class "metadata" ] [ text ("Response Location: " ++ location) ]

            Nothing ->
                text ""
        , if String.endsWith ".default" key && not (List.isEmpty check.responseHeaders) then
            details [ class "response-headers" ]
                [ summary [] [ text "Full response headers" ]
                , pre [] [ text (String.join "\n" check.responseHeaders) ]
                ]

          else
            text ""
        , if List.isEmpty check.redirectChain then
            text ""

          else
            details [ class "response-headers redirect-chain" ]
                [ summary [] [ text (redirectChainLabel check.redirectChain) ]
                , pre [] [ text (String.join "\n\n" (List.map redirectHopText check.redirectChain)) ]
                ]
        , ruleReferenceView (ruleReferences key check)
        ]


redirectChainLabel : List RedirectHop -> String
redirectChainLabel chain =
    let
        count =
            List.length chain
    in
    "Redirect chain — "
        ++ String.fromInt count
        ++ (if count == 1 then
                " hop"

            else
                " hops"
           )


redirectHopText : RedirectHop -> String
redirectHopText hop =
    String.join "\n"
        ([ "HTTP " ++ String.fromInt hop.httpStatus ++ " · " ++ hop.url ] ++ hop.responseHeaders)


corsDiagnosticRow : ( String, CheckResult ) -> Html msg
corsDiagnosticRow ( key, check ) =
    let
        preflight =
            String.contains "preflight" key

        negotiatedRequest =
            String.endsWith ".v2" key || String.endsWith ".v3" key

        displayStatus =
            if check.httpStatus == Nothing && check.status /= Unknown then
                Fail

            else if preflight then
                check.status

            else if check.httpStatus /= Nothing then
                Pass

            else
                Warning

        explanation =
            if preflight then
                check.summary

            else if check.httpStatus == Nothing then
                "The scheduled checker received no HTTP response, so CORS could not be confirmed."

            else if negotiatedRequest then
                "The scheduled checker received the negotiated GET response and recorded its CORS headers."

            else
                "The scheduled checker received the GET response and recorded its CORS headers."

        corsReferences =
            if String.contains "Access-Control-Allow-Headers does not include Accept" check.summary then
                [ ( "Fetch Standard — CORS-safelisted request headers", "https://fetch.spec.whatwg.org/#cors-safelisted-request-header" )
                , ( "MDN preflight requests", "https://developer.mozilla.org/en-US/docs/Glossary/Preflight_request" )
                ]

            else if preflight then
                [ ( "MDN preflight requests", "https://developer.mozilla.org/en-US/docs/Glossary/Preflight_request" ) ]

            else
                [ ( "MDN CORS", "https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS" ) ]
    in
    div [ class ("diagnostic-item cors-diagnostic diagnostic-" ++ statusClass displayStatus) ]
        [ div [ class "diagnostic-title" ]
            [ span [ class ("diagnostic-dot status-bg-" ++ statusClass displayStatus), attribute "aria-hidden" "true" ] []
            , span []
                [ text
                    (checkName key
                        ++ (if preflight then
                                ""

                            else
                                " — GET"
                           )
                    )
                ]
            , span [ class "http-code" ] [ text (Maybe.map (\code -> "HTTP " ++ String.fromInt code) check.httpStatus |> Maybe.withDefault "No response") ]
            ]
        , p [] [ text explanation ]
        , if List.isEmpty check.corsHeaders then
            p [ class "cors-headers cors-unavailable" ]
                [ text
                    (if check.httpStatus == Nothing then
                        "No CORS response headers were returned."

                     else
                        "No Access-Control-* response headers were returned."
                    )
                ]

          else
            div [ class "cors-headers" ]
                (p [ class "cors-heading" ] [ text "Returned CORS headers" ]
                    :: List.map
                        (\header -> p [ class "cors-header" ] [ text header ])
                        check.corsHeaders
                )
        , ruleReferenceView
            (checkPageReference key ++ corsReferences)
        ]


ruleReferenceView : List ( String, String ) -> Html msg
ruleReferenceView references =
    if List.isEmpty references then
        text ""

    else
        div [ class "rule-references" ]
            (span [ class "rule-references-label" ] [ text "References" ]
                :: List.intersperse (text " · ")
                    (List.map
                        (\( referenceLabel, referenceUrl ) ->
                            a [ href referenceUrl, target "_blank", rel "noopener noreferrer" ] [ text (referenceLabel ++ " ↗") ]
                        )
                        references
                    )
            )


ruleReferences : String -> CheckResult -> List ( String, String )
ruleReferences key check =
    checkPageReference key ++ externalRuleReferences key check


checkPageReference : String -> List ( String, String )
checkPageReference key =
    case key of
        "presentation.default" ->
            [ ( "How this check works", "./checks.html#presentation-default" ) ]

        "presentation.compression" ->
            [ ( "How this check works", "./checks.html#presentation-compression" ) ]

        "presentation.v2" ->
            [ ( "How this check works", "./checks.html#presentation-negotiation" ) ]

        "presentation.v3" ->
            [ ( "How this check works", "./checks.html#presentation-negotiation" ) ]

        "presentation.preflight" ->
            [ ( "How this check works", "./checks.html#presentation-preflight" ) ]

        "image.default" ->
            [ ( "How this check works", "./checks.html#image-default" ) ]

        "image.compression" ->
            [ ( "How this check works", "./checks.html#image-compression" ) ]

        "image.v2" ->
            [ ( "How this check works", "./checks.html#image-negotiation" ) ]

        "image.v3" ->
            [ ( "How this check works", "./checks.html#image-negotiation" ) ]

        "image.info-preflight" ->
            [ ( "How this check works", "./checks.html#image-preflight" ) ]

        "image.base-redirect" ->
            [ ( "How this check works", "./checks.html#image-base-redirect" ) ]

        "image.response" ->
            [ ( "How this check works", "./checks.html#image-response" ) ]

        "image.response-preflight" ->
            [ ( "How this check works", "./checks.html#image-response" ) ]

        _ ->
            []


externalRuleReferences : String -> CheckResult -> List ( String, String )
externalRuleReferences key check =
    case key of
        "presentation.default" ->
            if check.detected |> Maybe.map (String.startsWith "v2") |> Maybe.withDefault False then
                [ ( "Presentation API 2.1 §5.1", "https://iiif.io/api/presentation/2.1/#51-manifest" ) ]

            else if check.detected |> Maybe.map (String.startsWith "v3") |> Maybe.withDefault False then
                [ ( "Presentation API 3.0 §5.2", "https://iiif.io/api/presentation/3.0/#52-manifest" ) ]

            else
                [ ( "Presentation API 2.1 §5.1", "https://iiif.io/api/presentation/2.1/#51-manifest" )
                , ( "Presentation API 3.0 §5.2", "https://iiif.io/api/presentation/3.0/#52-manifest" )
                ]

        "presentation.v2" ->
            [ ( "Presentation API 2.1 §7.2", "https://iiif.io/api/presentation/2.1/#72-responses" )
            , ( "MDN Vary", "https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Vary" )
            ]

        "presentation.v3" ->
            [ ( "Presentation API 3.0 §6.3", "https://iiif.io/api/presentation/3.0/#63-responses" )
            , ( "MDN Vary", "https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Vary" )
            ]

        "presentation.compression" ->
            [ ( "Presentation API 3.0 §6.3", "https://iiif.io/api/presentation/3.0/#63-responses" )
            , ( "MDN Content-Encoding", "https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Encoding" )
            ]

        "presentation.preflight" ->
            [ ( "Fetch Standard — CORS-safelisted request headers", "https://fetch.spec.whatwg.org/#cors-safelisted-request-header" )
            , ( "MDN preflight requests", "https://developer.mozilla.org/en-US/docs/Glossary/Preflight_request" )
            ]

        "image.default" ->
            if check.detected |> Maybe.map (String.startsWith "v1") |> Maybe.withDefault False then
                [ ( "Image API 1.1", "https://iiif.io/api/image/1.1/" ) ]

            else if check.detected |> Maybe.map (String.startsWith "v2") |> Maybe.withDefault False then
                [ ( "Image API 2.1 §5.2", "https://iiif.io/api/image/2.1/#52-technical-properties" ) ]

            else if check.detected |> Maybe.map (String.startsWith "v3") |> Maybe.withDefault False then
                [ ( "Image API 3.0 §5.2", "https://iiif.io/api/image/3.0/#52-technical-properties" ) ]

            else
                [ ( "Image API 2.1 §5.2", "https://iiif.io/api/image/2.1/#52-technical-properties" )
                , ( "Image API 3.0 §5.2", "https://iiif.io/api/image/3.0/#52-technical-properties" )
                ]

        "image.v2" ->
            [ ( "Image API 2.1 §5.1", "https://iiif.io/api/image/2.1/#51-image-information-request" )
            , ( "MDN Vary", "https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Vary" )
            ]

        "image.v3" ->
            [ ( "Image API 3.0 §5.1", "https://iiif.io/api/image/3.0/#51-image-information-request" )
            , ( "MDN Vary", "https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Vary" )
            ]

        "image.compression" ->
            [ ( "MDN Content-Encoding", "https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Encoding" )
            , ( "MDN Vary", "https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Vary" )
            ]

        "image.info-preflight" ->
            [ ( "Image API 3.0 §7.1", "https://iiif.io/api/image/3.0/#71-cors" )
            , ( "Fetch Standard — CORS-safelisted request headers", "https://fetch.spec.whatwg.org/#cors-safelisted-request-header" )
            , ( "MDN preflight requests", "https://developer.mozilla.org/en-US/docs/Glossary/Preflight_request" )
            ]

        "image.base-redirect" ->
            [ ( "Image API 3.0 §2", "https://iiif.io/api/image/3.0/#2-uri-syntax" ) ]

        "image.response" ->
            [ ( "Image API 3.0 §4", "https://iiif.io/api/image/3.0/#4-image-requests" )
            , ( "Image API 3.0 §7.1", "https://iiif.io/api/image/3.0/#71-cors" )
            ]

        "image.response-preflight" ->
            [ ( "Image API 3.0 §7.1", "https://iiif.io/api/image/3.0/#71-cors" )
            , ( "MDN preflight requests", "https://developer.mozilla.org/en-US/docs/Glossary/Preflight_request" )
            ]

        _ ->
            []


checkName : String -> String
checkName key =
    case List.reverse (String.split "." key) |> List.head of
        Just "default" ->
            "Default response"

        Just "v2" ->
            "Request IIIF v2"

        Just "v3" ->
            "Request IIIF v3"

        Just "response" ->
            "Representative image"

        Just "preflight" ->
            "Manifest negotiation preflight"

        Just "info-preflight" ->
            "info.json negotiation preflight"

        Just "response-preflight" ->
            "Representative image OPTIONS preflight"

        Just "base-redirect" ->
            "Base URI → info.json redirect"

        Just "compression" ->
            "Gzip compression"

        _ ->
            key


viewErrors : List String -> Html msg
viewErrors errors =
    if List.isEmpty errors then
        text ""

    else
        div [ class "error-banner", attribute "role" "alert" ] (List.map (p [] << List.singleton << text) errors)


httpError : String -> Http.Error -> String
httpError resource error =
    "Could not load " ++ resource ++ ": " ++ httpErrorLabel error


httpErrorLabel : Http.Error -> String
httpErrorLabel error =
    case error of
        Http.BadUrl _ ->
            "invalid URL"

        Http.Timeout ->
            "request timed out"

        Http.NetworkError ->
            "network error"

        Http.BadStatus code ->
            "HTTP " ++ String.fromInt code

        Http.BadBody message ->
            "invalid response (" ++ message ++ ")"


friendlyStamp : String -> String
friendlyStamp stamp =
    String.replace "T" " " stamp |> String.replace ".000Z" " UTC" |> String.replace "Z" " UTC"


statusClass : Status -> String
statusClass status =
    case status of
        Pass ->
            "pass"

        Advisory ->
            "advisory"

        Warning ->
            "warning"

        Fail ->
            "fail"

        Unknown ->
            "unknown"


statusIcon : Status -> String
statusIcon status =
    case status of
        Pass ->
            "✓"

        Advisory ->
            "◆"

        Warning ->
            "!"

        Fail ->
            "×"

        Unknown ->
            "–"


boolString : Bool -> String
boolString value =
    if value then
        "true"

    else
        "false"
